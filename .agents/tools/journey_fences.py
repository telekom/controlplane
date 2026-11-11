#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Report non-Mermaid opening fences, including unlabelled Markdown fences."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path

FENCE = re.compile(r"^ {0,3}(`{3,}|~{3,})(.*)$")
QUOTE = re.compile(r"^ {0,3}> ?")
LIST = re.compile(r"^ {0,3}(?:[-+*]|\d{1,9}[.)])([ \t]+)")


def findings(path: Path) -> list[dict]:
    opened: tuple[str, int, tuple[tuple[str, int], ...]] | None = None
    containers: list[tuple[str, int]] = []
    findings = []
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        line = line.expandtabs(4)
        continued = []
        for kind, width in containers:
            if kind == "quote":
                quote = QUOTE.match(line)
                if not quote:
                    break
                line = line[quote.end():]
            else:
                indentation = len(line) - len(line.lstrip(" "))
                if line.strip() and indentation < width:
                    break
                line = line[min(width, indentation):]
            continued.append((kind, width))
        containers = continued
        if opened:
            marker_char, marker_length, fence_containers = opened
            if tuple(containers) != fence_containers:
                opened = None
            else:
                match = FENCE.match(line)
                if match:
                    marker, info = match.groups()
                    if marker[0] == marker_char and len(marker) >= marker_length and not info.strip():
                        opened = None
                continue
        while True:
            if quote := QUOTE.match(line):
                containers.append(("quote", 0))
                line = line[quote.end():]
            elif item := LIST.match(line):
                containers.append(("list", item.end()))
                line = line[item.end():]
            else:
                break
        match = FENCE.match(line)
        if not match:
            continue
        marker, info = match.groups()
        if marker[0] == "`" and "`" in info:
            continue
        opened = marker[0], len(marker), tuple(containers)
        if info.strip() != "mermaid":
            findings.append({
                "file": str(path), "line": number,
                "detail": "non-Mermaid fenced code block",
            })
    return findings


def report(path: Path) -> list[str]:
    return [f"{f['file']}:{f['line']}: {f['detail']}" for f in findings(path)]


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--jsonl", action="store_true")
    parser.add_argument("files", nargs="*")
    args = parser.parse_args()
    for name in args.files:
        if args.jsonl:
            for finding in findings(Path(name)):
                print(json.dumps(finding))
        else:
            for finding in report(Path(name)):
                print(finding)
