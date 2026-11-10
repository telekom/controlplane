# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Provenance for rule examples.

"Take examples from real code" is advice, and advice is not enforcement. A
provenance reference turns it into a check: every example block declares where
it came from, and the declaration is re-anchored against the repository at the
pinned revision.

Pinning the revision is what makes this stable. Violation examples describe
code the project wants gone, so a reference to a moving branch would rot the
moment someone fixes it. ``path:line@rev`` is immutable.

Info string grammar::

    ```go source=admin/internal/handler/zone/links.go:52@b53a2d9c
    ```go synthetic="no committed instance exists; this is what we prevent"
"""

from __future__ import annotations

import re
import shlex
import subprocess
from dataclasses import dataclass
from pathlib import Path

SOURCE_PATTERN = re.compile(r"^(?P<path>[^:@\s]+):(?P<line>\d+)@(?P<rev>[0-9a-fA-F]{7,40})$")
COMMENT_PREFIXES = ("//", "#", "<!--", "/*", "*")
ELISION = ("...", "// ...", "# ...")
WINDOW_BEFORE = 5
WINDOW_AFTER = 15


class ProvenanceError(ValueError):
    pass


@dataclass(frozen=True)
class Provenance:
    path: str
    line: int
    rev: str

    def __str__(self) -> str:
        return f"{self.path}:{self.line}@{self.rev}"


def parse_info(info: str) -> tuple[str, Provenance | None, str | None]:
    """Split a fence info string into (language, provenance, synthetic reason)."""
    try:
        tokens = shlex.split(info.strip())
    except ValueError as exc:
        raise ProvenanceError(f"malformed fence info string: {exc}") from exc
    if not tokens:
        return "", None, None

    language, attributes = tokens[0], tokens[1:]
    provenance: Provenance | None = None
    synthetic: str | None = None

    for token in attributes:
        key, _, value = token.partition("=")
        if key == "source":
            match = SOURCE_PATTERN.match(value)
            if not match:
                raise ProvenanceError(
                    f"source must read 'path:line@rev', got '{value}'"
                )
            provenance = Provenance(
                path=match.group("path"),
                line=int(match.group("line")),
                rev=match.group("rev"),
            )
        elif key == "synthetic":
            synthetic = value.strip()
            if not synthetic:
                raise ProvenanceError("synthetic needs a reason in quotes")
        else:
            raise ProvenanceError(f"unknown example attribute '{key}'")

    if provenance and synthetic:
        raise ProvenanceError("an example is either sourced or synthetic, not both")
    return language, provenance, synthetic


def _significant(code: str) -> list[str]:
    lines = []
    for raw in code.splitlines():
        text = raw.strip()
        if not text or text in ELISION or text.startswith(COMMENT_PREFIXES):
            continue
        lines.append(" ".join(text.split()))
    return lines


def _rev_available(root: Path, rev: str) -> bool:
    result = subprocess.run(
        ["git", "cat-file", "-e", f"{rev}^{{commit}}"],
        cwd=root,
        capture_output=True,
    )
    return result.returncode == 0


def verify(root: Path, provenance: Provenance, code: str) -> str | None:
    """Return an error message, or None when the example checks out.

    A revision that is not present locally (shallow clone) is reported as
    unavailable rather than as a failure, so CI checkout depth cannot turn an
    honest rule into a broken one.
    """
    if not _rev_available(root, provenance.rev):
        return f"UNAVAILABLE: revision {provenance.rev} is not in this clone"

    result = subprocess.run(
        ["git", "show", f"{provenance.rev}:{provenance.path}"],
        cwd=root,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        return f"{provenance}: path does not exist at that revision"

    haystack = result.stdout.splitlines()
    wanted = _significant(code)
    if not wanted:
        return f"{provenance}: example has no code to verify"

    start = max(1, provenance.line - WINDOW_BEFORE)
    end = min(len(haystack), provenance.line + len(wanted) + WINDOW_AFTER)
    window = [" ".join(line.split()) for line in haystack[start - 1 : end]]

    cursor = 0
    for needle in wanted:
        for index in range(cursor, len(window)):
            if needle in window[index]:
                cursor = index + 1
                break
        else:
            return (
                f"{provenance}: example line not found in lines {start}-{end} "
                f"of the pinned revision: {needle!r}"
            )
    return None
