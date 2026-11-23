#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Architecture decision records: validate, list, create, match and signal.

ADRs live in ``docs/docs/decisions/NNNN-slug.md`` and are read by people on
the documentation site and by agents during reviews. An agent that trusts a
wrong record repeats the mistake in every change it touches, so everything a
script can verify is verified here rather than left to a reviewer: evidence
must resolve, ``governs`` globs must still match files, supersede links must
agree in both directions, and a record must not carry code where a diagram or
a sentence would do.

Usage::

    adr.py validate                         # every ADR; exit 1 on errors
    adr.py list [--status accepted]         # one line per ADR
    adr.py new "Title" --domain gateway     # next-numbered skeleton
    adr.py match [--mode MODE] [PATH ...]   # ADRs governing a change or paths
    adr.py signals [--mode MODE]            # hints that a change may need an ADR

MODE is ``worktree`` (default: the branch plus uncommitted work against its
merge base), ``local`` (uncommitted work), ``staged`` or ``range``.
"""

from __future__ import annotations

import argparse
import datetime
import json
import re
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path

AUDITOR_SCRIPTS = Path(__file__).resolve().parents[2] / "code-auditor" / "scripts"
sys.path.insert(0, str(AUDITOR_SCRIPTS))

from auditlib import changeset  # noqa: E402
from auditlib.rules import glob_to_regex  # noqa: E402
from auditlib.yamlish import YamlishError, split_frontmatter  # noqa: E402

DECISIONS_DIR = Path("docs") / "docs" / "decisions"
NON_ADR_FILES = {"index.md", "_category_.json"}
FILENAME = re.compile(r"^(\d{4})-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$")

STATUSES = ("proposed", "accepted", "rejected", "deprecated", "superseded")
#: Statuses whose record no longer describes the system; never matched.
RETIRED = ("deprecated", "superseded")
ORIGINS = ("authored", "backfilled")
REQUIRED_KEYS = ("title", "status", "date", "origin", "domains")
LIST_KEYS = ("domains", "governs", "enforced-by", "evidence", "supersedes")
KNOWN_KEYS = REQUIRED_KEYS + LIST_KEYS[1:] + ("superseded-by",)
ALL_DOMAINS = "all"

SECTIONS = ("Context", "Decision", "Options considered", "Consequences")
REQUIRED_SECTIONS = SECTIONS[:3]
MIN_OPTIONS = 2
#: An honest record of a decision taken without weighing alternatives.
NO_ALTERNATIVES = "No alternatives were evaluated."
WORD_BUDGET = 500

DATE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
COMMIT_REF = re.compile(r"^[0-9a-f]{7,40}$")
URL = re.compile(r"^https?://\S+$")
FENCE = re.compile(r"^\s*(`{3,}|~{3,})\s*([\w-]*)")
CODE_BLOCK_REASON = re.compile(r"^\s*<!--\s*code-block:\s*\S.*-->\s*$")
HEADING = re.compile(r"^(#{1,6})\s+(.*?)\s*#*\s*$")
LIST_ITEM = re.compile(r"^\s*(?:[-*+]|\d+\.)\s+\S")
PLACEHOLDER = "<!-- adr:"

#: Mermaid sizes a label before the browser lays it out; anything much past
#: its default 200px wrapping width is cut off in some fonts and zoom levels.
MERMAID_LINE_MAX = 24
MERMAID_SKIP = re.compile(r"^\s*(%%|classDef\b|class\b|style\b|linkStyle\b|direction\b)")
MERMAID_LABELS = (
    re.compile(r'"([^"]*)"'),
    re.compile(r"\|([^|]*)\|"),
    re.compile(r"[\[({]+([^\[\](){}\"|]+)[\])}]+"),
    re.compile(r"^\s*\S+\s*-->\s*\S+\s*:\s*(.+)$"),
)
LINE_BREAK = re.compile(r"<br\s*/?>", re.IGNORECASE)
HTML_TAG = re.compile(r"<[^>]+>")


def mermaid_label_lines(line: str) -> list[str]:
    """The rendered lines of every label on one mermaid source line."""
    if MERMAID_SKIP.match(line):
        return []
    found: list[str] = []
    rest = line
    for pattern in MERMAID_LABELS:
        for match in pattern.finditer(rest):
            for part in LINE_BREAK.split(match.group(1)):
                text = HTML_TAG.sub("", part).strip()
                if text:
                    found.append(text)
        rest = pattern.sub(" ", rest)
    return found

MODULE_GRAPH = ".github/ci/module-graph.json"
OWN_MODULE_PREFIX = "github.com/telekom/controlplane"
KIND_MARKER = "+kubebuilder:object:root=true"
API_SOURCE = glob_to_regex("**/api/**/*.go")
GENERATED_SOURCE = glob_to_regex("**/zz_generated*.go")
GO_MOD = glob_to_regex("**/go.mod")

WORKTREE = "<worktree>"
INDEX = "<index>"
MODES = ("worktree", "local", "staged", "range")

SKELETON = """\
---
title: "{title}"
status: proposed
date: {date}
origin: {origin}
domains: [{domains}]
governs: []
evidence: []
---

## Context

<!-- adr: Why did this need deciding? Name the problem, the forces and the constraints in plain words. -->

## Decision

<!-- adr: What was decided and why it wins. Add a mermaid diagram when the decision has a shape. -->

## Options considered

<!-- adr: One bullet per option, the chosen one first. "**Option** (chosen): why", then "**Option**: why not". -->

## Consequences

<!-- adr: Optional. What becomes easier or harder. Delete this section when nothing is non-obvious. -->
"""


@dataclass
class Adr:
    path: str
    number: int
    meta: dict
    body: str

    @property
    def ref(self) -> str:
        return f"ADR-{self.number:04d}"

    @property
    def title(self) -> str:
        return str(self.meta.get("title", ""))

    @property
    def status(self) -> str:
        return str(self.meta.get("status", ""))

    @property
    def origin(self) -> str:
        return str(self.meta.get("origin", ""))

    def items(self, key: str) -> list:
        value = self.meta.get(key, [])
        return value if isinstance(value, list) else []


@dataclass
class Findings:
    errors: list[str] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)

    def error(self, where: str, message: str) -> None:
        self.errors.append(f"{where}: {message}")

    def warn(self, where: str, message: str) -> None:
        self.warnings.append(f"{where}: {message}")


# --- loading -----------------------------------------------------------------


def load(root: Path, findings: Findings | None = None) -> list[Adr]:
    """Every ADR that parses, sorted by number. Problems go to ``findings``."""
    findings = findings or Findings()
    adrs: list[Adr] = []
    directory = root / DECISIONS_DIR
    if not directory.is_dir():
        return adrs
    for path in sorted(directory.iterdir()):
        relative = str(path.relative_to(root))
        if path.name in NON_ADR_FILES or path.is_dir():
            continue
        match = FILENAME.match(path.name)
        if match is None:
            findings.error(relative, "ADR files are named NNNN-kebab-case-slug.md")
            continue
        try:
            meta, body = split_frontmatter(path.read_text(encoding="utf-8"))
        except (OSError, UnicodeDecodeError, YamlishError) as exc:
            findings.error(relative, str(exc))
            continue
        adrs.append(Adr(path=relative, number=int(match.group(1)), meta=meta, body=body))
    return adrs


def tracked_files(root: Path) -> list[str]:
    return [p for p in changeset.git(root, "ls-files", "-z").split("\0") if p]


def domains(root: Path) -> set[str]:
    return {
        p.name for p in root.iterdir() if p.is_dir() and not p.name.startswith(".")
    } | {ALL_DOMAINS}


def number_of(value) -> int | None:
    text = str(value).strip().upper().removeprefix("ADR-")
    return int(text) if text.isdigit() else None


# --- validate ----------------------------------------------------------------


def validate(root: Path) -> Findings:
    findings = Findings()
    adrs = load(root, findings)
    files = tracked_files(root)
    known_domains = domains(root)
    by_number: dict[int, Adr] = {}

    for adr in adrs:
        if adr.number in by_number:
            findings.error(adr.path, f"duplicate number, also used by {by_number[adr.number].path}")
        by_number.setdefault(adr.number, adr)

    for adr in adrs:
        _validate_meta(root, adr, findings, files, known_domains)
        _validate_links(adr, by_number, findings)
        _validate_body(adr, findings)
    return findings


def _validate_meta(
    root: Path, adr: Adr, findings: Findings, files: list[str], known_domains: set[str]
) -> None:
    where, meta = adr.path, adr.meta
    for key in sorted(set(meta) - set(KNOWN_KEYS)):
        findings.error(where, f"unknown frontmatter key '{key}'")
    for key in REQUIRED_KEYS:
        if meta.get(key) in (None, "", []):
            findings.error(where, f"missing required '{key}'")
    for key in LIST_KEYS:
        if key in meta and not isinstance(meta[key], list):
            findings.error(where, f"'{key}' must be a list")
        for item in adr.items(key):
            if item == "":
                findings.error(where, f"empty '{key}' item")

    if meta.get("status") and adr.status not in STATUSES:
        findings.error(where, f"status must be one of {', '.join(STATUSES)}")
    if meta.get("origin") and adr.origin not in ORIGINS:
        findings.error(where, f"origin must be one of {', '.join(ORIGINS)}")
    if meta.get("date") and not DATE.match(str(meta["date"])):
        findings.error(where, "date must be YYYY-MM-DD")

    for domain in adr.items("domains"):
        if domain and str(domain) not in known_domains:
            findings.error(
                where, f"unknown domain '{domain}'; use a top-level directory or '{ALL_DOMAINS}'"
            )

    if adr.status not in RETIRED:
        for pattern in adr.items("governs"):
            if pattern and not any(glob_to_regex(str(pattern)).match(f) for f in files):
                findings.error(where, f"governs '{pattern}' matches no tracked file")

    for path in adr.items("enforced-by"):
        if path and not (root / str(path)).exists():
            findings.error(where, f"enforced-by '{path}' does not exist")

    if adr.origin == "backfilled" and not adr.items("evidence"):
        findings.error(where, "a backfilled ADR must cite its evidence")
    for item in adr.items("evidence"):
        problem = _evidence_problem(root, str(item))
        if item and problem:
            findings.error(where, f"evidence '{item}' {problem}")


def _evidence_problem(root: Path, item: str) -> str | None:
    if URL.match(item):
        return None
    if COMMIT_REF.match(item):
        result = subprocess.run(
            ["git", "cat-file", "-e", f"{item}^{{commit}}"],
            cwd=root, capture_output=True,
        )
        return None if result.returncode == 0 else "is not a commit in this repository"
    path = item.split("#", 1)[0]
    if path and (root / path).exists():
        return None
    return "is neither a URL, a commit in this repository nor an existing path"


def _validate_links(adr: Adr, by_number: dict[int, Adr], findings: Findings) -> None:
    successor = adr.meta.get("superseded-by")
    if adr.status == "superseded" and successor in (None, ""):
        findings.error(adr.path, "a superseded ADR must name 'superseded-by'")
    if successor not in (None, ""):
        if adr.status != "superseded":
            findings.error(adr.path, "'superseded-by' requires status 'superseded'")
        target = by_number.get(number_of(successor) or -1)
        if target is None:
            findings.error(adr.path, f"superseded-by '{successor}' is not an ADR")
        elif adr.number not in {number_of(n) for n in target.items("supersedes")}:
            findings.error(adr.path, f"{target.ref} does not list {adr.ref} in 'supersedes'")

    for value in adr.items("supersedes"):
        target = by_number.get(number_of(value) or -1)
        if target is None:
            findings.error(adr.path, f"supersedes '{value}' is not an ADR")
        elif adr.status == "accepted" and number_of(target.meta.get("superseded-by")) != adr.number:
            # A proposed successor leaves its predecessor in force until a human accepts it.
            findings.error(
                adr.path,
                f"{target.ref} must have status superseded and superseded-by {adr.number:04d}",
            )


def _validate_body(adr: Adr, findings: Findings) -> None:
    where = adr.path
    sections: list[str] = []
    options = 0
    no_alternatives = False
    words = 0
    fence: str | None = None
    in_mermaid = False
    previous = ""

    for line in adr.body.splitlines():
        stripped = line.strip()
        fence_match = FENCE.match(line)
        if fence is not None:
            closing = fence_match and not fence_match.group(2)
            if closing and fence_match.group(1).startswith(fence):
                fence = None
            elif in_mermaid:
                for text in mermaid_label_lines(line):
                    if len(text) > MERMAID_LINE_MAX:
                        findings.error(
                            where,
                            f"diagram label line '{text}' has {len(text)} characters; keep each "
                            f"line at most {MERMAID_LINE_MAX} and break longer labels with <br/>, "
                            "or the text is cut off when rendered",
                        )
            previous = line
            continue
        if fence_match:
            fence = fence_match.group(1)
            in_mermaid = fence_match.group(2) == "mermaid"
            if not in_mermaid and not CODE_BLOCK_REASON.match(previous):
                findings.error(
                    where,
                    "code blocks are reserved for mermaid diagrams; explain in prose, or justify "
                    "the block with a '<!-- code-block: reason -->' line right above it",
                )
            previous = line
            continue
        if PLACEHOLDER in line:
            findings.error(where, "unfinished section: replace every '<!-- adr: ... -->' hint")
        heading = HEADING.match(line)
        if heading:
            level, text = len(heading.group(1)), heading.group(2)
            if level == 1:
                findings.error(where, "no '#' heading; the frontmatter title is the page title")
            elif level == 2:
                sections.append(text)
                if text not in SECTIONS:
                    findings.error(
                        where, f"unknown section '## {text}'; use {', '.join(SECTIONS)}"
                    )
        elif sections and sections[-1] == "Options considered" and LIST_ITEM.match(line):
            options += 1
        elif sections and sections[-1] == "Options considered" and stripped.startswith(NO_ALTERNATIVES):
            no_alternatives = True
        if stripped and not stripped.startswith("<!--"):
            words += len(stripped.split())
        previous = line

    if fence is not None:
        findings.error(where, "unterminated code block")
    known = [s for s in sections if s in SECTIONS]
    for required in REQUIRED_SECTIONS:
        if required not in known:
            findings.error(where, f"missing section '## {required}'")
    if known != sorted(known, key=SECTIONS.index) or len(known) != len(set(known)):
        findings.error(where, f"sections must appear once, in the order {', '.join(SECTIONS)}")
    if "Options considered" in known and no_alternatives and options != 1:
        findings.error(
            where,
            f"'{NO_ALTERNATIVES}' needs exactly the chosen option in 'Options considered', "
            f"found {options}",
        )
    elif "Options considered" in known and not no_alternatives and options < MIN_OPTIONS:
        findings.error(
            where,
            f"'Options considered' lists {options} option(s); a decision needs the chosen "
            "option and at least one alternative that was really weighed. If none was, say "
            f"so with a line starting '{NO_ALTERNATIVES}'",
        )
    if words > WORD_BUDGET:
        findings.warn(
            where,
            f"{words} words of prose (budget {WORD_BUDGET}); move the how into the "
            "architecture docs and link to it",
        )


# --- new ---------------------------------------------------------------------


def slugify(title: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", title.lower()).strip("-")
    return slug[:60].rstrip("-") or "decision"


def create(root: Path, title: str, domain_names: list[str], backfilled: bool) -> Path:
    if '"' in title:
        raise ValueError("titles cannot contain double quotes")
    unknown = sorted(set(domain_names) - domains(root))
    if unknown:
        raise ValueError(f"unknown domain(s): {', '.join(unknown)}")
    directory = root / DECISIONS_DIR
    # Numbers come from filenames so an ADR that fails to parse still holds its number.
    taken = [
        int(match.group(1))
        for match in (FILENAME.match(p.name) for p in directory.glob("*.md"))
        if match
    ]
    number = max(taken, default=0) + 1
    path = directory / f"{number:04d}-{slugify(title)}.md"
    directory.mkdir(parents=True, exist_ok=True)
    path.write_text(
        SKELETON.format(
            title=title,
            date=datetime.date.today().isoformat(),
            origin="backfilled" if backfilled else "authored",
            domains=", ".join(domain_names),
        ),
        encoding="utf-8",
    )
    return path


# --- match -------------------------------------------------------------------


def _base_and_head(root: Path, mode: str, base: str | None, revision: str) -> tuple[str, str]:
    if mode == "staged":
        return base or "HEAD", INDEX
    if mode == "local":
        return base or "HEAD", WORKTREE
    if mode == "worktree":
        return base or changeset.merge_base(root, revision), WORKTREE
    if mode == "range":
        return base or changeset.merge_base(root, revision), revision
    raise ValueError(f"unknown mode: {mode}")


def changed_paths(root: Path, mode: str, base: str | None, revision: str) -> list[str]:
    """Changed paths including deletions; removing governed code matters too."""
    base_rev, head = _base_and_head(root, mode, base, revision)
    if head == INDEX:
        args = ["diff", "--cached", "--name-only", "-z", base_rev]
    elif head == WORKTREE:
        args = ["diff", "--name-only", "-z", base_rev]
    else:
        args = ["diff", "--name-only", "-z", base_rev, head]
    paths = set(filter(None, changeset.git(root, *args).split("\0")))
    if head == WORKTREE:
        untracked = changeset.git(root, "ls-files", "-z", "--others", "--exclude-standard")
        paths |= set(filter(None, untracked.split("\0")))
    return sorted(paths)


def expand(root: Path, paths: list[str]) -> list[str]:
    """Explicit paths, with directories expanded to the tracked files below them."""
    result: set[str] = set()
    for raw in paths:
        path = raw.rstrip("/")
        if (root / path).is_dir():
            listed = changeset.git(root, "ls-files", "-z", "--", path)
            result |= set(filter(None, listed.split("\0")))
        else:
            result.add(path)
    return sorted(result)


def match(adrs: list[Adr], paths: list[str]) -> tuple[list[dict], list[str]]:
    """ADRs whose ``governs`` matches any path, and the paths nothing governs."""
    order = {"accepted": 0, "rejected": 1, "proposed": 2}
    governed: set[str] = set()
    found: list[dict] = []
    for adr in adrs:
        if adr.status in RETIRED:
            continue
        patterns = [glob_to_regex(str(p)) for p in adr.items("governs") if p]
        hits = [p for p in paths if any(rx.match(p) for rx in patterns)]
        if not hits:
            continue
        governed |= set(hits)
        found.append(
            {
                "ref": adr.ref,
                "title": adr.title,
                "status": adr.status,
                "origin": adr.origin,
                "path": adr.path,
                "enforced_by": [str(p) for p in adr.items("enforced-by")],
                "matched": hits,
            }
        )
    found.sort(key=lambda item: (order.get(item["status"], 3), item["ref"]))
    return found, [p for p in paths if p not in governed]


# --- signals -----------------------------------------------------------------


def read_at(root: Path, path: str, where: str) -> str | None:
    if where == WORKTREE:
        target = root / path
        return target.read_text(encoding="utf-8") if target.is_file() else None
    spec = f":{path}" if where == INDEX else f"{where}:{path}"
    try:
        return changeset.git(root, "show", spec)
    except changeset.GitError:
        return None


def direct_requires(text: str | None) -> set[str]:
    """External modules a go.mod requires directly."""
    found: set[str] = set()
    in_block = False
    for raw in (text or "").splitlines():
        line = raw.strip()
        if line.startswith("require ("):
            in_block = True
            continue
        if in_block and line == ")":
            in_block = False
            continue
        if line.startswith("require "):
            line = line[len("require "):].strip()
        elif not in_block:
            continue
        if not line or line.startswith("//") or "// indirect" in line:
            continue
        module = line.split()[0]
        if not module.startswith(OWN_MODULE_PREFIX):
            found.add(module)
    return found


def _graph(text: str | None) -> dict[str, set[str]]:
    try:
        raw = json.loads(text or "{}")
    except json.JSONDecodeError:
        return {}
    return {name: set(entry.get("depends_on", [])) for name, entry in raw.items()}


def signals(root: Path, mode: str, base: str | None, revision: str) -> list[dict]:
    """Deterministic hints that a change may carry an architectural decision."""
    base_rev, head = _base_and_head(root, mode, base, revision)
    paths = changed_paths(root, mode, base_rev, revision)
    found: list[dict] = []

    if MODULE_GRAPH in paths:
        before = _graph(read_at(root, MODULE_GRAPH, base_rev))
        after = _graph(read_at(root, MODULE_GRAPH, head))
        for module in sorted(set(after) - set(before)):
            found.append({"signal": "new-module", "path": MODULE_GRAPH, "detail": module})
        for module in sorted(set(after) & set(before)):
            for dependency in sorted(after[module] - before[module]):
                found.append(
                    {
                        "signal": "new-domain-dependency",
                        "path": MODULE_GRAPH,
                        "detail": f"{module} now depends on {dependency}",
                    }
                )

    if any(GO_MOD.match(path) for path in paths):
        listed = changeset.git(root, "ls-tree", "-r", "-z", "--name-only", base_rev)
        known: set[str] = set()
        for path in filter(None, listed.split("\0")):
            if GO_MOD.match(path):
                known |= direct_requires(read_at(root, path, base_rev))
        for path in paths:
            if not GO_MOD.match(path):
                continue
            added = direct_requires(read_at(root, path, head)) - known
            for module in sorted(added):
                found.append(
                    {"signal": "new-external-dependency", "path": path, "detail": module}
                )
                known.add(module)

    for file in changeset.collect(root, mode, base_rev, revision):
        if not API_SOURCE.match(file.path) or GENERATED_SOURCE.match(file.path):
            continue
        lines = (read_at(root, file.path, head) or "").splitlines()
        for number in file.added_lines:
            if number <= len(lines) and KIND_MARKER in lines[number - 1]:
                kind = _kind_after(lines, number)
                if kind and not kind.endswith("List"):
                    found.append({"signal": "new-api-kind", "path": file.path, "detail": kind})
    return found


def _kind_after(lines: list[str], marker_line: int) -> str | None:
    for line in lines[marker_line : marker_line + 10]:
        match = re.match(r"^type\s+(\w+)\s+struct\b", line)
        if match:
            return match.group(1)
    return None


# --- cli ---------------------------------------------------------------------


def _print_match(found: list[dict], ungoverned: list[str], total: int) -> None:
    for item in found:
        status = item["status"]
        if status == "proposed":
            status += ", unverified: not a constraint"
        print(f"{item['ref']} [{status}] {item['title']}")
        print(f"  {item['path']}")
        if item["enforced_by"]:
            print(f"  enforced by: {', '.join(item['enforced_by'])}")
        shown = item["matched"][:5]
        more = len(item["matched"]) - len(shown)
        print(f"  governs: {', '.join(shown)}" + (f", +{more} more" if more else ""))
    print(f"{total - len(ungoverned)} of {total} path(s) governed by an ADR.")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("--root", type=Path, help=argparse.SUPPRESS)
    commands = parser.add_subparsers(dest="command", required=True)

    check = commands.add_parser("validate", help="validate every ADR")
    check.add_argument("paths", nargs="*", help="ignored; accepted for pre-commit")

    listing = commands.add_parser("list", help="list ADRs")
    listing.add_argument("--status", action="append", choices=STATUSES)

    new = commands.add_parser("new", help="create the next-numbered ADR skeleton")
    new.add_argument("title")
    new.add_argument("--domain", action="append", required=True, help="repeatable")
    new.add_argument("--backfilled", action="store_true", help="record an existing decision")

    for name, help_text in (
        ("match", "ADRs governing a change or the given paths"),
        ("signals", "hints that a change may need an ADR"),
    ):
        sub = commands.add_parser(name, help=help_text)
        sub.add_argument("--mode", choices=MODES, default="worktree")
        sub.add_argument("--base", help="base revision; defaults to the merge base")
        sub.add_argument("--revision", default="HEAD", help="head revision for range mode")
        sub.add_argument("--json", action="store_true")
        if name == "match":
            sub.add_argument("paths", nargs="*", help="paths or directories instead of a change")

    args = parser.parse_args(argv)
    root = args.root or changeset.repo_root()

    try:
        if args.command == "validate":
            findings = validate(root)
            for warning in findings.warnings:
                print(f"warning: {warning}", file=sys.stderr)
            for error in findings.errors:
                print(f"error: {error}", file=sys.stderr)
            if findings.errors:
                print(
                    f"adr: {len(findings.errors)} error(s); see "
                    ".agents/skills/review-architecture/ADR-FORMAT.md",
                    file=sys.stderr,
                )
                return 1
            print(f"adr: {len(load(root))} ADR(s) valid.")
            return 0

        if args.command == "list":
            for adr in load(root):
                if args.status and adr.status not in args.status:
                    continue
                domain_list = ", ".join(str(d) for d in adr.items("domains"))
                print(f"{adr.ref}  {adr.status:<10} {adr.origin:<10} {adr.title}  [{domain_list}]")
            return 0

        if args.command == "new":
            path = create(root, args.title, args.domain, args.backfilled)
            print(path.relative_to(root))
            return 0

        if args.command == "match":
            paths = (
                expand(root, args.paths)
                if args.paths
                else changed_paths(root, args.mode, args.base, args.revision)
            )
            found, ungoverned = match(load(root), paths)
            if args.json:
                print(json.dumps({"adrs": found, "ungoverned": ungoverned}, indent=2))
            else:
                _print_match(found, ungoverned, len(paths))
            return 0

        if args.command == "signals":
            found = signals(root, args.mode, args.base, args.revision)
            if args.json:
                print(json.dumps(found, indent=2))
            elif not found:
                print("No ADR signals. Judge the change against the three criteria anyway.")
            else:
                for item in found:
                    print(f"{item['signal']}: {item['detail']}  ({item['path']})")
            return 0
    except (changeset.GitError, ValueError) as exc:
        print(f"adr: {exc}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
