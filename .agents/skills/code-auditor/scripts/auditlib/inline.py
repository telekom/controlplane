# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Inline audit ignores: a human marks a false positive next to the code.

A marker is a comment that starts with ``audit:ignore``, names the rules (and
optionally the checks) it ignores, and says why. It normally sits on its own
line above the code, so the code line does not grow::

    // audit:ignore 01-prefer-slices-sort byAge satisfies a third-party
    // sort.Interface that the vendor library requires.
    sort.Sort(byAge(items))

A marker alone on its line covers itself, the comment lines directly below it
that use the same comment opener (stacked markers, a wrapped reason), and the
first line after them. A marker at the end of a code line covers that line
only::

    x := BAD // audit:ignore 03-spelling,07-normalize-label-values short reason

Unlike ``.agents/auditignore`` a marker moves with the code it annotates, so
unrelated edits above it do not retire it. It is as visible in review as the
code it excuses, which is the trace an ignore must leave.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import PurePosixPath
from typing import Iterable

MARKER = "audit:ignore"

# The marker must open a comment, so prose that merely mentions it is inert.
_MARKER = re.compile(r"(?P<opener>//|/\*|<!--|#)[ \t]*audit:ignore(?![\w:/-])")
_CLOSERS = {"/*": "*/", "<!--": "-->"}
_TARGET = re.compile(
    r"(?P<rule>\d{2,}-[a-z0-9]+(?:-[a-z0-9]+)*)(?:/(?P<check>[a-z0-9][a-z0-9-]*))?"
)
_HASH_COMMENTS = {".yaml", ".yml", ".py", ".sh", ".toml"}
_HASH_COMMENT_NAMES = {"Makefile", "Dockerfile"}
_HTML_COMMENTS = {".md", ".mdx"}


@dataclass(frozen=True)
class InlineIgnore:
    line: int
    #: ``(rule_id, check_slug)`` pairs; an empty slug covers every check.
    targets: tuple[tuple[str, str], ...]
    reason: str
    #: Why the marker cannot be honored. A malformed marker never ignores.
    problem: str = ""

    def ignores(self, rule_id: str, check_id: str) -> bool:
        return not self.problem and any(
            rule == rule_id and check in ("", check_id) for rule, check in self.targets
        )

    def names(self, rule_id: str) -> bool:
        """Whether a malformed marker was plausibly meant for this rule."""
        return not self.targets or any(rule == rule_id for rule, _ in self.targets)

    def describe(self) -> dict:
        if self.problem:
            return {"line": self.line, "problem": self.problem}
        return {"line": self.line, "reason": self.reason}


def _parse(number: int, text: str) -> tuple[InlineIgnore, str] | None:
    """The marker on a line, plus its comment opener if it stands alone."""
    match = _MARKER.search(text)
    if match is None:
        return None
    rest = text[match.end():]
    closer = _CLOSERS.get(match["opener"])
    if closer:
        rest = rest.split(closer, 1)[0]
    parts = rest.split(None, 1)
    token = parts[0] if parts else ""
    reason = parts[1].strip() if len(parts) > 1 else ""

    targets: list[tuple[str, str]] = []
    problem = ""
    for target in token.split(",") if token else ():
        found = _TARGET.fullmatch(target)
        if found is None:
            problem = f"'{target}' is not a rule id or rule-id/check"
            targets = []
            break
        targets.append((found["rule"], found["check"] or ""))
    if not token:
        problem = f"{MARKER} names no rule"
    elif not problem and not reason:
        problem = f"{MARKER} {token} needs a reason"

    standalone = text[:match.start()].strip() in ("", "{")
    opener = match["opener"] if standalone else ""
    return InlineIgnore(number, tuple(targets), reason, problem), opener


class InlineIgnores:
    """Every marker in one file, indexed by the lines it covers."""

    def __init__(self, text: str):
        self._covering: dict[int, list[InlineIgnore]] = {}
        if MARKER not in text:
            return
        lines = text.splitlines()
        parsed = {
            number: found
            for number, line in enumerate(lines, start=1)
            if (found := _parse(number, line)) is not None
        }
        for number, (marker, opener) in parsed.items():
            self._covering.setdefault(number, []).append(marker)
            if not opener:
                continue
            # A marker above the code reaches past the comment lines below
            # it (stacked markers, a wrapped reason) to the first other line.
            target = number + 1
            while target <= len(lines) and (
                (target in parsed and parsed[target][1])
                or lines[target - 1].lstrip().startswith(opener)
            ):
                self._covering.setdefault(target, []).append(marker)
                target += 1
            if target <= len(lines):
                self._covering.setdefault(target, []).append(marker)

    def find(self, rule_id: str, check_id: str, lines: Iterable[int]) -> InlineIgnore | None:
        """The marker ignoring this check on any of ``lines``.

        Without one, return a malformed marker meant for the rule so the
        caller can say why it was not honored; otherwise ``None``.
        """
        malformed = None
        for line in lines:
            for marker in self._covering.get(line, ()):
                if marker.ignores(rule_id, check_id):
                    return marker
                if malformed is None and marker.problem and marker.names(rule_id):
                    malformed = marker
        return malformed


def hint(path: str, rule_id: str, check_id: str) -> str:
    """The marker a human would write to ignore this finding in ``path``."""
    target = rule_id if check_id == "violation" else f"{rule_id}/{check_id}"
    body = f"{MARKER} {target} <reason>"
    name = PurePosixPath(path)
    if name.suffix in _HTML_COMMENTS:
        return f"<!-- {body} -->"
    if name.suffix in _HASH_COMMENTS or name.name in _HASH_COMMENT_NAMES:
        return f"# {body}"
    return f"// {body}"
