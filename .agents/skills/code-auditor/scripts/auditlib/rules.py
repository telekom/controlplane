# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Parsing, validation and indexing of the rule corpus in .agents/rules."""

from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass, field
from pathlib import Path

from . import SEVERITIES
from .provenance import Provenance, ProvenanceError, parse_info
from .yamlish import YamlishError, split_frontmatter

RULES_DIRNAME = "rules"
ID_PATTERN = re.compile(r"^\d{2,}-[a-z0-9]+(?:-[a-z0-9]+)*$")
CHECK_PATTERN = re.compile(
    r"^\s*[-*]\s+\[(?P<slug>[a-z0-9][a-z0-9-]*)\]\s+"
    r"(?:when=`(?P<when>[^`]+)`\s+)?"
    r"(?P<question>.+?)\s*$"
)
HEADING_PATTERN = re.compile(r"^(#{2,3})\s+(.+?)\s*$")

COMPLEXITIES = (0, 1, 2)
SCOPES = ("line", "file", "changeset")
REPORT_SCOPES = ("changed", "file")
DETECTOR_KINDS = ("regex", "tool")
VIOLATION_WHEN = ("output_lines", "nonzero_exit")


class RuleError(ValueError):
    """Raised when a rule file does not satisfy the authoring contract."""


@dataclass(frozen=True)
class Detector:
    kind: str
    pattern: str = ""
    command: str = ""
    violation_when: str = "output_lines"
    flags: str = ""


@dataclass(frozen=True)
class Check:
    slug: str
    question: str
    #: Optional pattern that must appear in a candidate's context for this
    #: check to be worth asking. Without it the check always runs.
    when: str = ""

    def applies_to_context(self, context: str) -> bool:
        if not self.when:
            return True
        return re.search(self.when, context) is not None


@dataclass(frozen=True)
class Example:
    kind: str
    language: str
    code: str
    provenance: Provenance | None = None
    synthetic_reason: str = ""

    @property
    def is_sourced(self) -> bool:
        return self.provenance is not None


@dataclass(frozen=True)
class Rule:
    id: str
    path: Path
    description: str
    severity: str
    complexity: int
    scope: str
    report_scope: str
    applies_to: tuple[str, ...]
    exclude: tuple[str, ...]
    context_lines: int
    body: str
    content_hash: str
    detector: Detector | None = None
    checks: tuple[Check, ...] = ()
    examples: dict[str, tuple[Example, ...]] = field(default_factory=dict)

    @property
    def decides_without_llm(self) -> bool:
        return self.complexity == 0


def glob_to_regex(pattern: str) -> re.Pattern[str]:
    """Translate a gitignore-style glob into a regex anchored at both ends."""
    out, index, length = ["^"], 0, len(pattern)
    while index < length:
        char = pattern[index]
        if char == "*":
            if pattern.startswith("**/", index):
                out.append("(?:.*/)?")
                index += 3
                continue
            if pattern.startswith("**", index):
                out.append(".*")
                index += 2
                continue
            out.append("[^/]*")
        elif char == "?":
            out.append("[^/]")
        else:
            out.append(re.escape(char))
        index += 1
    out.append("$")
    return re.compile("".join(out))


class Matcher:
    def __init__(self, applies_to, exclude):
        self._include = [glob_to_regex(p) for p in applies_to]
        self._exclude = [glob_to_regex(p) for p in exclude]

    def matches(self, path: str) -> bool:
        if not any(rx.match(path) for rx in self._include):
            return False
        return not any(rx.match(path) for rx in self._exclude)


def _sections(body: str) -> dict[str, list[str]]:
    """Split a rule body into its level-2 sections, keyed by lowercased title."""
    sections: dict[str, list[str]] = {}
    current: str | None = None
    for line in body.splitlines():
        match = HEADING_PATTERN.match(line)
        if match and len(match.group(1)) == 2:
            current = match.group(2).strip().lower()
            sections[current] = []
            continue
        if current is not None:
            sections[current].append(line)
    return sections


FENCE_PATTERN = re.compile(r"^(?P<fence>`{3,})(?P<info>.*)$")


def _fenced_blocks(lines: list[str]) -> list[tuple[str, str]]:
    """Return (info string, content) for every fenced block in ``lines``.

    Fence length is tracked so that a documentation example can wrap a nested
    code block, which journey-doc rules need.
    """
    blocks, info, buffer = [], "", []
    fence = ""
    for line in lines:
        match = FENCE_PATTERN.match(line)
        if fence:
            if match and match.group("fence") >= fence and not match.group("info").strip():
                blocks.append((info, "\n".join(buffer)))
                buffer, fence = [], ""
                continue
            buffer.append(line)
            continue
        if match:
            fence, info = match.group("fence"), match.group("info").strip()
    return blocks


def _parse_detector(lines: list[str], rule_id: str) -> Detector:
    blocks = _fenced_blocks(lines)
    if len(blocks) != 1:
        raise RuleError(f"{rule_id}: '## Detect' must hold exactly one fenced block")
    kind, content = blocks[0]
    kind = kind.strip()
    if kind not in DETECTOR_KINDS:
        raise RuleError(
            f"{rule_id}: unknown detector kind '{kind}', expected one of {DETECTOR_KINDS}"
        )
    if kind == "regex":
        pattern = content.strip()
        if not pattern:
            raise RuleError(f"{rule_id}: empty regex detector")
        try:
            re.compile(pattern)
        except re.error as exc:
            raise RuleError(f"{rule_id}: invalid detector regex: {exc}") from exc
        return Detector(kind="regex", pattern=pattern)

    from .yamlish import parse as parse_yamlish

    try:
        config = parse_yamlish(content)
    except YamlishError as exc:
        raise RuleError(f"{rule_id}: invalid tool detector block: {exc}") from exc
    command = str(config.get("command", "")).strip()
    if not command:
        raise RuleError(f"{rule_id}: tool detector needs a 'command'")
    when = str(config.get("violation_when", "output_lines")).strip()
    if when not in VIOLATION_WHEN:
        raise RuleError(
            f"{rule_id}: violation_when must be one of {VIOLATION_WHEN}, got '{when}'"
        )
    return Detector(kind="tool", command=command, violation_when=when)


def _parse_checks(lines: list[str], rule_id: str) -> tuple[Check, ...]:
    checks, seen = [], set()
    for line in lines:
        if not line.strip() or not line.lstrip().startswith(("-", "*")):
            continue
        match = CHECK_PATTERN.match(line)
        if not match:
            raise RuleError(
                f"{rule_id}: check items must read '- [slug] question' with an "
                f"optional 'when=`regex`', got: {line.strip()!r}"
            )
        slug = match.group("slug")
        question = match.group("question")
        when = match.group("when") or ""
        if slug in seen:
            raise RuleError(f"{rule_id}: duplicate check slug '{slug}'")
        if not question.endswith("?"):
            raise RuleError(
                f"{rule_id}: check '{slug}' must be a closed question ending in '?'"
            )
        if when:
            try:
                re.compile(when)
            except re.error as exc:
                raise RuleError(
                    f"{rule_id}: check '{slug}' has an invalid when= pattern: {exc}"
                ) from exc
        seen.add(slug)
        checks.append(Check(slug=slug, question=question, when=when))
    return tuple(checks)


def _to_examples(kind: str, blocks: list[tuple[str, str]], rule_id: str) -> list[Example]:
    examples = []
    for info, code in blocks:
        try:
            language, provenance, synthetic = parse_info(info)
        except ProvenanceError as exc:
            raise RuleError(f"{rule_id}: {kind} example: {exc}") from exc
        if provenance is None and synthetic is None:
            raise RuleError(
                f"{rule_id}: {kind} example must declare where it came from: add "
                f"'source=path:line@rev' or 'synthetic=\"why\"' to the fence"
            )
        examples.append(
            Example(
                kind=kind,
                language=language,
                code=code,
                provenance=provenance,
                synthetic_reason=synthetic or "",
            )
        )
    return examples


def _parse_examples(lines: list[str], rule_id: str) -> dict[str, tuple[Example, ...]]:
    examples: dict[str, list[Example]] = {}
    current: str | None = None
    buffer: list[str] = []
    for line in lines:
        match = HEADING_PATTERN.match(line)
        if match and len(match.group(1)) == 3:
            if current:
                examples[current] = _to_examples(current, _fenced_blocks(buffer), rule_id)
            current = match.group(2).strip().lower()
            buffer = []
            continue
        buffer.append(line)
    if current:
        examples[current] = _to_examples(current, _fenced_blocks(buffer), rule_id)
    return {key: tuple(value) for key, value in examples.items()}


def parse_rule(path: Path) -> Rule:
    raw = path.read_text(encoding="utf-8")
    try:
        front, body = split_frontmatter(raw)
    except YamlishError as exc:
        raise RuleError(f"{path.name}: {exc}") from exc

    rule_id = str(front.get("id", "")).strip()
    if not rule_id:
        raise RuleError(f"{path.name}: missing 'id'")
    if not ID_PATTERN.match(rule_id):
        raise RuleError(
            f"{path.name}: id '{rule_id}' must look like '01-kebab-case-name'"
        )
    if path.name != f"{rule_id}.md":
        raise RuleError(f"{path.name}: filename must be '{rule_id}.md'")

    description = str(front.get("description", "")).strip()
    if not description:
        raise RuleError(f"{rule_id}: missing 'description'")

    severity = str(front.get("severity", "")).strip()
    if severity not in SEVERITIES:
        raise RuleError(f"{rule_id}: severity must be one of {SEVERITIES}")

    complexity = front.get("complexity")
    if complexity not in COMPLEXITIES:
        raise RuleError(f"{rule_id}: complexity must be one of {COMPLEXITIES}")

    scope = str(front.get("scope", "file")).strip()
    if scope not in SCOPES:
        raise RuleError(f"{rule_id}: scope must be one of {SCOPES}")

    report_scope = str(front.get("report_scope", "changed")).strip()
    if report_scope not in REPORT_SCOPES:
        raise RuleError(f"{rule_id}: report_scope must be one of {REPORT_SCOPES}")

    applies_to = tuple(str(p) for p in front.get("applies_to", []))
    if not applies_to:
        raise RuleError(f"{rule_id}: 'applies_to' must list at least one glob")
    exclude = tuple(str(p) for p in front.get("exclude", []))

    context_lines = front.get("context_lines", 20)
    if not isinstance(context_lines, int) or not 0 < context_lines <= 200:
        raise RuleError(f"{rule_id}: context_lines must be an int in 1..200")

    sections = _sections(body)
    detector = None
    if "detect" in sections:
        detector = _parse_detector(sections["detect"], rule_id)
    checks = _parse_checks(sections.get("checks", []), rule_id)
    examples = _parse_examples(sections.get("examples", []), rule_id)

    if complexity == 0:
        if detector is None:
            raise RuleError(f"{rule_id}: complexity 0 rules need a '## Detect' block")
        if checks:
            raise RuleError(
                f"{rule_id}: complexity 0 rules are decided by the detector and must not "
                f"declare '## Checks'"
            )
    else:
        if not checks:
            raise RuleError(
                f"{rule_id}: complexity {complexity} rules must decompose into '## Checks' items"
            )
        if complexity == 1 and detector is None:
            raise RuleError(
                f"{rule_id}: complexity 1 rules need a '## Detect' block; drop to complexity 2 "
                f"if no deterministic detector is possible"
            )
        if complexity == 2 and detector is not None:
            raise RuleError(
                f"{rule_id}: complexity 2 means 'no detector possible' but '## Detect' is "
                f"present; promote the rule to complexity 1"
            )
        if not examples.get("violation"):
            raise RuleError(
                f"{rule_id}: judgment rules must anchor on at least one "
                f"'### Violation' example"
            )
        every = [e for group in examples.values() for e in group]
        if not any(e.is_sourced for e in every):
            raise RuleError(
                f"{rule_id}: at least one example must cite real repository code "
                f"('source=path:line@rev'); a rule with only invented examples has "
                f"never been calibrated against this codebase"
            )

    if scope == "line" and detector is None:
        raise RuleError(f"{rule_id}: scope 'line' requires a detector")

    return Rule(
        id=rule_id,
        path=path,
        description=description,
        severity=severity,
        complexity=int(complexity),
        scope=scope,
        report_scope=report_scope,
        applies_to=applies_to,
        exclude=exclude,
        context_lines=int(context_lines),
        body=body.strip(),
        content_hash=hashlib.sha256(raw.encode("utf-8")).hexdigest()[:16],
        detector=detector,
        checks=checks,
        examples=examples,
    )


def rules_dir(repo_root: Path) -> Path:
    return repo_root / ".agents" / RULES_DIRNAME


def load_rules(repo_root: Path) -> list[Rule]:
    """Load and validate every rule, ordered by id. Raises on the first problem."""
    directory = rules_dir(repo_root)
    if not directory.is_dir():
        raise RuleError(f"rules directory not found: {directory}")

    rules, errors, seen_numbers = [], [], {}
    for path in sorted(directory.glob("*.md")):
        if path.name == "README.md":
            continue
        try:
            rule = parse_rule(path)
        except RuleError as exc:
            errors.append(str(exc))
            continue
        number = rule.id.split("-", 1)[0]
        if number in seen_numbers:
            errors.append(
                f"{rule.id}: duplicate rule number '{number}', already used by "
                f"'{seen_numbers[number]}'"
            )
        seen_numbers[number] = rule.id
        rules.append(rule)

    if errors:
        raise RuleError("\n".join(errors))
    if not rules:
        raise RuleError(f"no rules found in {directory}")
    return sorted(rules, key=lambda r: r.id)


def index_hash(rules: list[Rule]) -> str:
    digest = hashlib.sha256()
    for rule in rules:
        digest.update(rule.id.encode("utf-8"))
        digest.update(rule.content_hash.encode("utf-8"))
    return digest.hexdigest()[:16]


def load_index(repo_root: Path, cache_path: Path | None = None) -> list[Rule]:
    """Load rules, reusing a compiled index when the rules directory is unchanged.

    The cache only short-circuits the *staleness check*; rules are still parsed
    from disk so that every consumer sees identical objects.
    """
    directory = rules_dir(repo_root)
    fingerprint = hashlib.sha256()
    for path in sorted(directory.glob("*.md")):
        stat = path.stat()
        fingerprint.update(f"{path.name}:{stat.st_mtime_ns}:{stat.st_size}".encode())
    current = fingerprint.hexdigest()[:16]

    if cache_path and cache_path.is_file():
        try:
            cached = json.loads(cache_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            cached = {}
        if cached.get("fingerprint") == current:
            return load_rules(repo_root)

    rules = load_rules(repo_root)
    if cache_path:
        cache_path.parent.mkdir(parents=True, exist_ok=True)
        cache_path.write_text(
            json.dumps(
                {
                    "fingerprint": current,
                    "index_hash": index_hash(rules),
                    "rules": [
                        {
                            "id": r.id,
                            "severity": r.severity,
                            "complexity": r.complexity,
                            "content_hash": r.content_hash,
                        }
                        for r in rules
                    ],
                },
                indent=2,
            )
            + "\n",
            encoding="utf-8",
        )
    return rules
