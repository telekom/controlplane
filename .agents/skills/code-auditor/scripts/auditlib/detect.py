# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Candidate generation — the stage that keeps the engine cheap and repeatable.

Every candidate is produced by a script, never by a model. A rule that yields
no candidates is machine-passed and never reaches an agent.
"""

from __future__ import annotations

import hashlib
import json
import re
import shlex
import shutil
import subprocess
from dataclasses import dataclass, replace
from pathlib import Path

from .changeset import ChangedFile
from .prompt import context_budget
from .rules import Matcher, Rule


@dataclass(frozen=True)
class Candidate:
    rule_id: str
    file: str
    line: int
    anchor: str
    context: str
    detail: str = ""
    #: Declaration/comment span a tool reported, inclusive.
    span: tuple[int, int] | None = None

    @property
    def lines(self) -> range:
        """Every line a finding on this candidate stands for."""
        start, end = self.span or (self.line, self.line)
        return range(start, end + 1)

    @property
    def normalized_anchor(self) -> str:
        return " ".join(self.anchor.split())

    @property
    def occurrence_anchor(self) -> str:
        return f"{self.line}:{self.normalized_anchor}:{' '.join(self.detail.split())}"

    def cluster_key(self, rule: Rule) -> str:
        digest = hashlib.sha256()
        digest.update(rule.content_hash.encode("utf-8"))
        digest.update(b"\0")
        digest.update(self.judgment_key.encode("utf-8"))
        return digest.hexdigest()[:16]

    @property
    def judgment_key(self) -> str:
        return judgment_key(self.file, self.normalized_anchor, self.context, self.detail)


def judgment_key(path: str, anchor: str, context: str, detail: str) -> str:
    return hashlib.sha256(
        "\0".join((path, anchor, context, detail)).encode("utf-8")
    ).hexdigest()


class ToolUnavailable(RuntimeError):
    """A complexity 0 detector needs a binary that is not installed."""


def render_context(
    lines: list[str], line: int, radius: int, changed: frozenset[int]
) -> str:
    """Render a numbered window, marking which lines the change touched.

    The marker matters: without it a model reading a whole file has no way to
    tell the work under review from the code that was already there.
    """
    return render_range(lines, max(1, line - radius), min(len(lines), line + radius), changed)


def render_range(lines: list[str], start: int, end: int, changed: frozenset[int]) -> str:
    """Render lines ``start..end`` (1-based, inclusive), numbered and marked."""
    width = len(str(end))
    return "\n".join(
        f"{'>' if number in changed else ' '}{number:>{width}} | {lines[number - 1]}"
        for number in range(start, end + 1)
    )


def truncation_note(start: int, end: int, total: int) -> str:
    return (
        f"[truncated: showing lines {start}-{end} of {total}. "
        "Read other lines with the viewer's --file and --lines options.]"
    )


def fit_ranges(
    lines: list[str],
    start: int,
    end: int,
    changed: frozenset[int],
    budget: int,
    anchor: int | None = None,
) -> list[tuple[int, int, str]]:
    """Fit lines ``start..end`` into ``budget`` bytes as contiguous ranges.

    The range is returned unchanged when it fits. Otherwise it is cut to one
    contiguous range around the changed lines it contains and ``anchor``, grown towards both
    ends until the budget is full - near the top of a file that is simply
    lines 1 to n. If the changed lines alone do not fit, they are split into
    consecutive runs, one range each, so every changed line reaches a judge.

    A truncated range starts with a note naming the lines shown and the file's
    length, so the judge knows what it has not seen and can read it.
    Returns ``(first, last, context)`` per range.
    """
    full = render_range(lines, start, end, changed)
    if _fits(full, budget):
        return [(start, end, full)]

    total = len(lines)
    width = len(str(total))
    note = len(truncation_note(total, total, total).encode("utf-8")) + 1
    cost = [
        len(f"{'>'}{n:>{width}} | {lines[n - 1]}".encode("utf-8")) + 1
        for n in range(1, total + 1)
    ]

    def size(first: int, last: int) -> int:
        return note + sum(cost[first - 1:last])

    required = sorted(
        {n for n in range(start, end + 1) if n in changed}
        | ({anchor} if anchor and start <= anchor <= end else set())
    ) or [start]
    runs: list[list[int]] = []
    for number in required:
        if runs and size(runs[-1][0], number) <= budget:
            runs[-1].append(number)
        else:
            runs.append([number])

    out = []
    for run in runs:
        first, last = run[0], run[-1]
        used = size(first, last)
        while True:
            grew = False
            if first > start and used + cost[first - 2] <= budget:
                first -= 1
                used += cost[first - 1]
                grew = True
            if last < end and used + cost[last] <= budget:
                last += 1
                used += cost[last - 1]
                grew = True
            if not grew:
                break
        text = render_range(lines, first, last, changed)
        out.append((first, last, f"{truncation_note(first, last, total)}\n{text}"))
    return out


def _fits(text: str, budget: int) -> bool:
    return len(text.encode("utf-8")) <= budget


def _candidates_for(
    candidate: Candidate,
    lines: list[str],
    start: int,
    end: int,
    changed: frozenset[int],
    budget: int,
) -> list[Candidate]:
    """The candidate with its fitted context; one per range when it splits."""
    start, end = max(1, start), min(len(lines), end)
    if not lines or start > end:
        return [candidate]
    ranges = fit_ranges(lines, start, end, changed, budget, anchor=candidate.line)
    if len(ranges) == 1:
        return [replace(candidate, context=ranges[0][2])]
    split = []
    for index, (first, last, context) in enumerate(ranges, start=1):
        line = candidate.line if first <= candidate.line <= last else next(
            (n for n in range(first, last + 1) if n in changed), first
        )
        note = f"part {index} of {len(ranges)}: lines {first}-{last}"
        split.append(replace(
            candidate,
            line=line,
            anchor=candidate.anchor if line == candidate.line else lines[line - 1].strip(),
            context=context,
            detail=f"{candidate.detail}; {note}" if candidate.detail else note,
        ))
    return split


def _scan_lines(rule: Rule, file: ChangedFile, total: int):
    """Which lines of a file are eligible to produce a candidate."""
    if rule.report_scope == "file":
        return range(1, total + 1)
    if rule.complexity > 0:
        return sorted({
            number
            for changed in file.touched_lines
            for number in range(
                max(1, changed - rule.context_lines),
                min(total, changed + rule.context_lines) + 1,
            )
        })
    return file.touched_lines


def _regex_candidates(
    rule: Rule, file: ChangedFile, content: str, budget: int
) -> list[Candidate]:
    pattern = re.compile(rule.detector.pattern)
    lines = content.splitlines()
    changed = frozenset(file.touched_lines)

    found = []
    for number in _scan_lines(rule, file, len(lines)):
        if number > len(lines):
            continue
        text = lines[number - 1]
        if not pattern.search(text):
            continue
        found.append(
            Candidate(
                rule_id=rule.id,
                file=file.path,
                line=number,
                anchor=text.strip(),
                context="",
            )
        )

    if rule.scope == "file" and found:
        # One candidate per file. Anchor it on a line the change touched when
        # there is one, so the frozen window is centred on the work under
        # review rather than on whatever matched first in the file.
        head = next((c for c in found if c.line in changed), found[0])
        extra = ", ".join(f"line {c.line}" for c in found if c is not head)
        candidate = Candidate(
            rule_id=rule.id,
            file=file.path,
            line=head.line,
            anchor=head.anchor,
            context="",
            detail=f"also matched at {extra}" if extra else "",
        )
        return _candidates_for(candidate, lines, 1, len(lines), changed, budget)
    radius = rule.context_lines
    return [
        part
        for c in found
        for part in _candidates_for(
            c, lines, c.line - radius, c.line + radius, changed, budget
        )
    ]


def _tool_candidates(
    rule: Rule, files: list[ChangedFile], root: Path, contents: dict[str, str], budget: int
) -> list[Candidate]:
    argv = shlex.split(rule.detector.command)
    if not argv:
        raise ToolUnavailable(f"{rule.id}: empty detector command")
    # Config-owned scripts and data follow the rules; input paths and cwd
    # still belong to the repository being audited.
    for index, argument in enumerate(argv):
        option, separator, value = argument.partition("=")
        path = value if separator else argument
        if path.startswith(".agents/"):
            target = rule.path.resolve().parent.parent / path.removeprefix(".agents/")
            if not target.exists():
                raise ToolUnavailable(
                    f"{rule.id}: detector path '{target}' does not exist"
                )
            argv[index] = f"{option}={target}" if separator else str(target)
    if shutil.which(argv[0]) is None:
        raise ToolUnavailable(
            f"{rule.id}: detector tool '{argv[0]}' is not installed"
        )

    paths = [f.path for f in files]
    result = subprocess.run(
        [*argv, *paths], cwd=root, capture_output=True, text=True
    )
    output = result.stdout

    if rule.detector.violation_when == "nonzero_exit":
        if result.returncode == 0:
            return []
        first = files[0]
        return [
            Candidate(
                rule_id=rule.id,
                file=first.path,
                line=1,
                anchor=f"{argv[0]} exited {result.returncode}",
                context=(output + result.stderr)[:4000],
                detail=(output + result.stderr)[:4000],
            )
        ]

    # codespell uses 65 for findings. Other failures, including invocation
    # errors and Python tracebacks, must never turn into "no candidates".
    accepted = {0, 65} if Path(argv[0]).name == "codespell" else {0}
    if result.returncode not in accepted:
        raise ToolUnavailable(
            f"{rule.id}: detector exited {result.returncode}: "
            f"{(result.stderr or output).strip()[:4000]}"
        )

    candidates = []
    known = {f.path: f for f in files}
    text_pattern = re.compile(
        r"^(?P<file>" + "|".join(re.escape(p) for p in sorted(known, key=len, reverse=True))
        + r"):(?P<line>\d+):(?P<rest>.*)$"
    )
    for line in output.split("\n"):
        if not line:
            continue
        if rule.detector.output_format == "jsonl":
            try:
                record = json.loads(line)
            except json.JSONDecodeError as exc:
                raise ToolUnavailable(f"{rule.id}: malformed JSON detector output: {exc}") from exc
            if (
                not isinstance(record, dict)
                or not isinstance(record.get("file"), str)
                or type(record.get("line")) is not int
                or not isinstance(record.get("detail"), str)
            ):
                raise ToolUnavailable(f"{rule.id}: invalid detector record: {record!r}")
            path, number, detail = record["file"], record["line"], record["detail"]
            span_value = record.get("span")
            if span_value is not None and (
                not isinstance(span_value, list) or len(span_value) != 2
                or any(type(n) is not int for n in span_value)
            ):
                raise ToolUnavailable(f"{rule.id}: invalid detector span: {span_value!r}")
        else:
            match = text_pattern.fullmatch(line)
            if not match:
                raise ToolUnavailable(f"{rule.id}: malformed detector output: {line!r}")
            path, number, detail = match["file"], int(match["line"]), match["rest"].strip()
            span = re.search(r" \[audit-span=(\d+)-(\d+)\]$", detail)
            span_value = [int(span[1]), int(span[2])] if span else None
            if span:
                detail = detail[:span.start()]
        changed_file = known.get(path)
        if changed_file is None:
            raise ToolUnavailable(f"{rule.id}: detector reported an unknown path: {path!r}")
        body = contents.get(path, "").splitlines()
        if not 1 <= number <= len(body):
            raise ToolUnavailable(f"{rule.id}: detector line outside file: {path!r}:{number}")
        if span_value is not None and not 1 <= span_value[0] <= number <= span_value[1] <= len(body):
            raise ToolUnavailable(f"{rule.id}: detector span outside file: {span_value!r}")
        changed = frozenset(changed_file.touched_lines)
        # A tool reports on whole files. Without this filter, touching one line
        # of a file makes you the owner of every pre-existing hit in it.
        eligible = {number}
        if span_value:
            eligible = set(range(span_value[0], span_value[1] + 1))
        if rule.report_scope == "changed" and not eligible.intersection(changed):
            continue
        if number not in changed and eligible.intersection(changed):
            number = min(eligible.intersection(changed))
        anchor = body[number - 1].strip() if 0 < number <= len(body) else ""
        candidate = Candidate(
            rule_id=rule.id,
            file=path,
            line=number,
            anchor=anchor,
            context="",
            detail=detail,
            span=(span_value[0], span_value[1]) if span_value else None,
        )
        first, last = span_value or (number, number)
        candidates.extend(
            _candidates_for(
                candidate,
                body,
                first - rule.context_lines,
                last + rule.context_lines,
                changed,
                budget,
            )
        )
    return candidates


def _whole_file_candidates(
    rule: Rule, file: ChangedFile, content: str, budget: int
) -> list[Candidate]:
    lines = content.splitlines()
    if not lines:
        return []
    changed = frozenset(file.touched_lines)
    anchor_line = min(changed) if changed else 1
    candidate = Candidate(
        rule_id=rule.id,
        file=file.path,
        line=anchor_line,
        anchor=f"{file.path}:{hashlib.sha256(content.encode('utf-8')).hexdigest()[:16]}",
        context="",
        detail=f"{len(changed)} changed line(s), marked '>' in the context",
    )
    return _candidates_for(candidate, lines, 1, len(lines), changed, budget)


def applicable_files(rule: Rule, files: list[ChangedFile]) -> list[ChangedFile]:
    matcher = Matcher(rule.applies_to, rule.exclude)
    return [f for f in files if matcher.matches(f.path)]


def locates_findings(rule: Rule) -> bool:
    """Whether each candidate is one finding's location, not a whole file.

    Only then can a marker on the candidate settle it before judgment; a
    file-wide candidate is settled by where its finding lands instead.
    """
    if rule.complexity == 0:
        return True
    if rule.detector is None:
        return False
    if rule.detector.kind == "tool":
        return rule.detector.violation_when == "output_lines"
    return rule.scope != "file"


def run(
    rule: Rule,
    files: list[ChangedFile],
    root: Path,
    contents: dict[str, str],
) -> list[Candidate]:
    """Return every candidate the rule's detector produces, sorted stably."""
    scoped = applicable_files(rule, files)
    if not scoped:
        return []
    budget = context_budget(rule)

    if rule.detector is None:
        candidates: list[Candidate] = []
        for file in scoped:
            candidates.extend(
                _whole_file_candidates(rule, file, contents.get(file.path, ""), budget)
            )
    elif rule.detector.kind == "tool":
        candidates = _tool_candidates(rule, scoped, root, contents, budget)
    else:
        candidates = []
        for file in scoped:
            candidates.extend(
                _regex_candidates(rule, file, contents.get(file.path, ""), budget)
            )

    return sorted(candidates, key=lambda c: (c.file, c.line, c.anchor))
