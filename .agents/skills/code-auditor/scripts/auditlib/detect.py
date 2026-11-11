# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Candidate generation — the stage that keeps the engine cheap and repeatable.

Every candidate is produced by a script, never by a model. A rule that yields
no candidates is machine-passed and never reaches an agent.
"""

from __future__ import annotations

import hashlib
import re
import shlex
import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path

from .changeset import ChangedFile
from .rules import Matcher, Rule

TOOL_OUTPUT_PATTERN = re.compile(r"^(?P<file>[^:]+):(?P<line>\d+):(?P<rest>.*)$")


@dataclass(frozen=True)
class Candidate:
    rule_id: str
    file: str
    line: int
    anchor: str
    context: str
    detail: str = ""

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
    start = max(1, line - radius)
    end = min(len(lines), line + radius)
    width = len(str(end))
    return "\n".join(
        f"{'>' if number in changed else ' '}{number:>{width}} | {lines[number - 1]}"
        for number in range(start, end + 1)
    )


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


def _regex_candidates(rule: Rule, file: ChangedFile, content: str) -> list[Candidate]:
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
                context=render_context(lines, number, rule.context_lines, changed),
            )
        )

    if rule.scope == "file" and found:
        # One candidate per file. Anchor it on a line the change touched when
        # there is one, so the frozen window is centred on the work under
        # review rather than on whatever matched first in the file.
        head = next((c for c in found if c.line in changed), found[0])
        extra = ", ".join(f"line {c.line}" for c in found if c is not head)
        return [
            Candidate(
                rule_id=rule.id,
                file=file.path,
                line=head.line,
                anchor=head.anchor,
                context=render_context(
                    lines, 1, len(lines), changed
                ),
                detail=f"also matched at {extra}" if extra else "",
            )
        ]
    return found


def _tool_candidates(
    rule: Rule, files: list[ChangedFile], root: Path, contents: dict[str, str]
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
    output = result.stdout.strip()

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
    for line in output.splitlines():
        match = TOOL_OUTPUT_PATTERN.match(line.strip())
        if not match:
            continue
        path = match.group("file")
        changed_file = known.get(path)
        if changed_file is None:
            continue
        number = int(match.group("line"))
        changed = frozenset(changed_file.touched_lines)
        # A tool reports on whole files. Without this filter, touching one line
        # of a file makes you the owner of every pre-existing hit in it.
        detail = match.group("rest").strip()
        span = re.search(r" \[audit-span=(\d+)-(\d+)\]$", detail)
        eligible = {number}
        if span:
            eligible = set(range(int(span[1]), int(span[2]) + 1))
            detail = detail[:span.start()]
        if rule.report_scope == "changed" and not eligible.intersection(changed):
            continue
        if number not in changed and eligible.intersection(changed):
            number = min(eligible.intersection(changed))
        body = contents.get(path, "").splitlines()
        anchor = body[number - 1].strip() if 0 < number <= len(body) else ""
        candidates.append(
            Candidate(
                rule_id=rule.id,
                file=path,
                line=number,
                anchor=anchor,
                context=render_context(body, number, rule.context_lines, changed)
                if body
                else "",
                detail=detail,
            )
        )
    return candidates


def _whole_file_candidates(
    rule: Rule, file: ChangedFile, content: str
) -> list[Candidate]:
    lines = content.splitlines()
    if not lines:
        return []
    changed = frozenset(file.touched_lines)
    anchor_line = min(changed) if changed else 1
    width = len(str(len(lines)))
    numbered = "\n".join(
        f"{'>' if number in changed else ' '}{number:>{width}} | {text}"
        for number, text in enumerate(lines, start=1)
    )
    return [
        Candidate(
            rule_id=rule.id,
            file=file.path,
            line=anchor_line,
            anchor=f"{file.path}:{hashlib.sha256(content.encode('utf-8')).hexdigest()[:16]}",
            context=numbered,
            detail=f"{len(changed)} changed line(s), marked '>' in the context",
        )
    ]


def applicable_files(rule: Rule, files: list[ChangedFile]) -> list[ChangedFile]:
    matcher = Matcher(rule.applies_to, rule.exclude)
    return [f for f in files if matcher.matches(f.path)]


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

    if rule.detector is None:
        candidates: list[Candidate] = []
        for file in scoped:
            candidates.extend(
                _whole_file_candidates(rule, file, contents.get(file.path, ""))
            )
    elif rule.detector.kind == "tool":
        candidates = _tool_candidates(rule, scoped, root, contents)
    else:
        candidates = []
        for file in scoped:
            candidates.extend(
                _regex_candidates(rule, file, contents.get(file.path, ""))
            )

    return sorted(candidates, key=lambda c: (c.file, c.line, c.anchor))
