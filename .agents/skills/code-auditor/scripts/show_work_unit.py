#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Print exactly one work unit: its rule, its candidates and its questions.

A subagent must never read the whole report. Beyond the token cost, the items
it is not responsible for are context that can only pull its judgment around.

A candidate's context may be truncated to fit what a judge can receive. The
judge can then read more of a candidate's file, from the same frozen snapshot
the audit was made of, with ``--file`` and ``--lines``.

Usage::

    show_work_unit.py <unit_id> [--report PATH] [--config-root DIR]
    show_work_unit.py <unit_id> --file PATH --lines START-END [--config-root DIR]
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from auditlib.context import ConfigError, add_argument, resolve  # noqa: E402
from auditlib.detect import render_range  # noqa: E402
from auditlib.prompt import MAX_UNIT_BYTES, render_unit  # noqa: E402

LINES = re.compile(r"^(\d+)-(\d+)$")


def show_lines(report: dict, items: list[dict], root: Path, path: str, span: str) -> int:
    """Print a line range of one candidate file, as frozen by the audit."""
    match = LINES.fullmatch(span)
    if not match or not 1 <= int(match[1]) <= int(match[2]):
        print(f"--lines must be START-END with 1 <= START <= END, got {span!r}", file=sys.stderr)
        return 2
    if path not in {i["file"] for i in items}:
        known = ", ".join(sorted({i["file"] for i in items}))
        print(f"{path} is not a file of this work unit. Files: {known}", file=sys.stderr)
        return 2
    frozen = next((f for f in report["scope"]["files"] if f["path"] == path), None)
    if frozen is None:
        print(f"No frozen snapshot of {path} in this report.", file=sys.stderr)
        return 2
    result = subprocess.run(
        ["git", "cat-file", "blob", frozen["blob_sha"]], cwd=root, capture_output=True
    )
    if result.returncode:
        print(f"Cannot read the frozen {path}: {result.stderr.decode().strip()}", file=sys.stderr)
        return 2
    lines = result.stdout.decode("utf-8", errors="replace").splitlines()
    total = len(lines)
    start, end = int(match[1]), min(int(match[2]), total)
    if start > total:
        print(f"{path} has only {total} lines.", file=sys.stderr)
        return 2
    changed = frozenset(frozen.get("changed_lines", frozen.get("added_lines", [])))

    shown, used = start - 1, 0
    for number in range(start, end + 1):
        used += len(render_range(lines, number, number, changed).encode("utf-8")) + 1
        if used > MAX_UNIT_BYTES:
            break
        shown = number
    shown = max(shown, start)
    print(f"# {path}: lines {start}-{shown} of {total} ('>' marks lines this change touched)")
    print(render_range(lines, start, shown, changed))
    if shown < end:
        print(f"# stopped at line {shown} to stay readable; request {shown + 1}-{end} next")
    return 0



def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("unit_id")
    parser.add_argument("--report", type=Path, default=None)
    parser.add_argument("--file", default=None, help="a file of this unit's candidates")
    parser.add_argument("--lines", default=None, help="START-END line range of --file")
    add_argument(parser)
    args = parser.parse_args()

    if args.report:
        path = args.report
    else:
        try:
            directory = resolve(args.config_root).audit_dir
            path = (
                directory / "selftest/plan.json"
                if args.unit_id.startswith("selftest-")
                else directory / "report.json"
            )
        except ConfigError as exc:
            print(f"Cannot show work unit: {exc}", file=sys.stderr)
            return 2
    if not path.is_file():
        print(f"No report at {path}. Generate the audit or selftest plan first.", file=sys.stderr)
        return 2
    report = json.loads(path.read_text(encoding="utf-8"))

    unit = next((u for u in report["work_units"] if u["unit_id"] == args.unit_id), None)
    if unit is None:
        known = ", ".join(u["unit_id"] for u in report["work_units"]) or "none"
        print(f"No work unit '{args.unit_id}'. Available: {known}", file=sys.stderr)
        return 2

    rule = report["rule_prompts"][unit["rule_id"]]
    wanted = set(unit["item_ids"])
    items = [i for i in report["items"] if i["item_id"] in wanted]

    if (args.file is None) != (args.lines is None):
        print("--file and --lines must be given together.", file=sys.stderr)
        return 2
    if args.file is not None:
        try:
            root = resolve(args.config_root).repo_root
        except ConfigError as exc:
            print(f"Cannot show lines: {exc}", file=sys.stderr)
            return 2
        return show_lines(report, items, root, args.file, args.lines)

    sys.stdout.write(render_unit(unit["unit_id"], rule, items))
    return 0


if __name__ == "__main__":
    sys.exit(main())
