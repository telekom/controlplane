#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Print exactly one work unit: its rule, its candidates and its questions.

A subagent must never read the whole report. Beyond the token cost, the items
it is not responsible for are context that can only pull its judgment around.

Usage::

    show_work_unit.py <unit_id> [--report PATH] [--config-root DIR]
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from auditlib.context import ConfigError, add_argument, resolve  # noqa: E402

#: Sections that are re-rendered with more structure further down, or that
#: only concern the engine. Repeating them wastes context and invites the
#: agent to re-run the detector in its head.
MACHINE_SECTIONS = ("## detect", "## checks", "## examples")


def _prose(body: str) -> str:
    kept = []
    for line in body.splitlines():
        if line.strip().lower() in MACHINE_SECTIONS:
            break
        kept.append(line)
    return "\n".join(kept).strip()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("unit_id")
    parser.add_argument("--report", type=Path, default=None)
    add_argument(parser)
    args = parser.parse_args()

    if args.report:
        path = args.report
    else:
        try:
            path = resolve(args.config_root).audit_dir / "report.json"
        except ConfigError as exc:
            print(f"Cannot show work unit: {exc}", file=sys.stderr)
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

    print(f"# Work unit {unit['unit_id']}")
    print(f"# Rule {rule['id']} [{rule['severity']}] - {rule['description']}")
    print()
    print(_prose(rule["body"]))
    print()

    for kind in ("violation", "allowed"):
        for example in rule["examples"][kind]:
            print(f"## Example: {kind}  (from {example['source']})")
            print(example["code"])
            print()

    by_candidate: dict[str, list[dict]] = {}
    for item in items:
        by_candidate.setdefault(item["candidate_id"], []).append(item)

    for index, (_, group) in enumerate(sorted(by_candidate.items()), start=1):
        head = group[0]
        print(f"## Candidate {index}: {head['file']}:{head['line']}")
        if head["detail"]:
            print(f"({head['detail']})")
        print()
        print("### Context ('>' marks lines this change touched)")
        print(head["context"])
        print()
        print("### Questions")
        for item in sorted(group, key=lambda i: i["check_id"]):
            print(f"- item_id={item['item_id']}  [{item['check_id']}]")
            print(f"  {item['question']}")
        print()

    print(f"# Answer all {len(items)} item(s). Return JSON only.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
