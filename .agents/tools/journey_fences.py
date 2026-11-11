#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Report non-Mermaid opening fences, including unlabelled Markdown fences."""

from __future__ import annotations

import re
import sys
from pathlib import Path

FENCE = re.compile(r"^ {0,3}(`{3,}|~{3,})(.*)$")


def report(path: Path) -> list[str]:
    opened: tuple[str, int] | None = None
    findings = []
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        match = FENCE.match(line)
        if not match:
            continue
        marker, info = match.groups()
        if opened:
            if marker[0] == opened[0] and len(marker) >= opened[1] and not info.strip():
                opened = None
            continue
        if marker[0] == "`" and "`" in info:
            continue
        opened = marker[0], len(marker)
        if info.strip() != "mermaid":
            findings.append(f"{path}:{number}: non-Mermaid fenced code block")
    return findings


if __name__ == "__main__":
    for name in sys.argv[1:]:
        for finding in report(Path(name)):
            print(finding)
