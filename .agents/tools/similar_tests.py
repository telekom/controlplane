#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Report Ginkgo It/Specify blocks whose body is a near-copy of another one in the same file."""

from __future__ import annotations

import argparse
import difflib
import json
import re
from pathlib import Path

START = re.compile(r"^\s*(?:It|Specify|FIt|FSpecify)\(")
STRING = re.compile(r'"(?:[^"\\\n]|\\.)*"|`[^`]*`')
COMMENT = re.compile(r"//[^\n]*")
NUMBER = re.compile(r"\b\d+(?:\.\d+)?\b")
TOKEN = re.compile(r"[A-Za-z_]\w*|\S")

MIN_TOKENS = 20
THRESHOLD = 0.95


def blocks(lines: list[str]) -> list[tuple[int, list[str]]]:
    found = []
    for start, line in enumerate(lines):
        if not START.match(line):
            continue
        depth, opened, body = 0, False, []
        for line_no in range(start, len(lines)):
            text = COMMENT.sub("", STRING.sub("S", lines[line_no]))
            depth += text.count("{") - text.count("}")
            opened = opened or "{" in text
            if line_no > start:
                body.append(text)
            if opened and depth <= 0:
                break
        tokens = TOKEN.findall(NUMBER.sub("N", "\n".join(body)))
        if len(tokens) >= MIN_TOKENS:
            found.append((start + 1, tokens))
    return found


def findings(path: Path) -> list[dict]:
    items = blocks(path.read_text(encoding="utf-8").splitlines())
    reported: dict[int, int] = {}
    for i, (line_a, tokens_a) in enumerate(items):
        for line_b, tokens_b in items[i + 1:]:
            matcher = difflib.SequenceMatcher(None, tokens_a, tokens_b, autojunk=False)
            if matcher.real_quick_ratio() < THRESHOLD or matcher.quick_ratio() < THRESHOLD:
                continue
            if matcher.ratio() < THRESHOLD:
                continue
            reported.setdefault(line_a, line_b)
            reported.setdefault(line_b, line_a)
    return [
        {"file": str(path), "line": line, "detail": f"test body nearly identical to the one at line {other}"}
        for line, other in sorted(reported.items())
    ]


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--jsonl", action="store_true")
    parser.add_argument("files", nargs="*")
    args = parser.parse_args()
    for name in args.files:
        for finding in findings(Path(name)):
            if args.jsonl:
                print(json.dumps(finding))
            else:
                print(f"{finding['file']}:{finding['line']}: {finding['detail']}")
