#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""The commit/push gate: prove an audit covers exactly what is being committed.

No model, no network, no credentials - just hashes. For every staged blob and
every rule that applies to it, a receipt must exist under the current rule
text. Edit a file after auditing and its blob changes, the receipt vanishes and
the gate blocks. That is the whole mechanism.

Tier 0 rules are re-run here directly: they are cheap, and catching them before
asking for a full audit is a better experience.

Usage::

    audit_gate.py [--mode staged|range] [--fail-on LEVEL]
"""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from auditlib import changeset, detect, rules as rulelib  # noqa: E402
from auditlib.judges import JudgeError  # noqa: E402
from auditlib.report import SEVERITY_RANK  # noqa: E402
from auditlib.store import (  # noqa: E402
    ReceiptStore,
    audit_dir,
    audited_model,
    receipt_key,
)

SKILL_HINT = "Run the code-auditor skill, then commit again."


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=("staged", "range"), default="staged")
    parser.add_argument(
        "--fail-on", choices=("LOW", "HIGH", "CRITICAL"), default="CRITICAL"
    )
    parser.add_argument("paths", nargs="*", help="ignored; accepted for pre-commit")
    args = parser.parse_args()

    if os.environ.get("AUDIT_GATE", "").lower() in ("off", "0", "false"):
        print("audit-gate: disabled via AUDIT_GATE")
        return 0

    root = changeset.repo_root()
    try:
        rules = rulelib.load_index(root, audit_dir(root) / "rules.index.json")
    except rulelib.RuleError as exc:
        print(f"audit-gate: the rule corpus is invalid.\n{exc}", file=sys.stderr)
        return 1

    mode = "staged" if args.mode == "staged" else "worktree"
    files = changeset.collect(root, mode=mode)
    if not files:
        return 0

    contents = {
        f.path: changeset.read_blob(root, f, staged=(mode == "staged")) for f in files
    }

    threshold = SEVERITY_RANK[args.fail_on]
    try:
        model = audited_model(root)
    except JudgeError as exc:
        print(f"audit-gate: cannot tell which judge to expect.\n{exc}", file=sys.stderr)
        return 2
    receipts = ReceiptStore(root)

    tier0: list[str] = []
    missing: dict[str, set[str]] = {}
    blocked: list[str] = []

    for rule in rules:
        scoped = detect.applicable_files(rule, files)
        if not scoped:
            continue

        if rule.tier == 0:
            try:
                for candidate in detect.run(rule, files, root, contents):
                    if SEVERITY_RANK[rule.severity] >= threshold:
                        tier0.append(
                            f"[{rule.severity}] {rule.id}  {candidate.file}:"
                            f"{candidate.line}  {candidate.detail or candidate.anchor}"
                        )
            except detect.ToolUnavailable as exc:
                print(f"audit-gate: skipping {rule.id}: {exc}", file=sys.stderr)
            continue

        for file in scoped:
            receipt = receipts.get(receipt_key(file.blob_sha, rule.content_hash, model))
            if receipt is None:
                missing.setdefault(file.path, set()).add(rule.id)
            elif (
                receipt["verdict"] in ("FAIL", "UNCLEAR")
                and SEVERITY_RANK[rule.severity] >= threshold
            ):
                blocked.append(
                    f"[{rule.severity}] {rule.id}  {file.path}  "
                    f"verdict={receipt['verdict']}"
                )

    if not tier0 and not missing and not blocked:
        print(f"audit-gate: {len(files)} file(s) covered by a current audit.")
        return 0

    print("audit-gate: commit blocked.\n", file=sys.stderr)
    if tier0:
        print("  Deterministic rule violations:", file=sys.stderr)
        for line in sorted(tier0):
            print(f"    {line}", file=sys.stderr)
        print(file=sys.stderr)
    if blocked:
        print("  Unresolved findings from the last audit:", file=sys.stderr)
        for line in sorted(blocked):
            print(f"    {line}", file=sys.stderr)
        print(file=sys.stderr)
    if missing:
        print("  Changed since the last audit (no receipt):", file=sys.stderr)
        for path in sorted(missing):
            rule_ids = ", ".join(sorted(missing[path]))
            print(f"    {path}  ({rule_ids})", file=sys.stderr)
        print(file=sys.stderr)

    print(f"  {SKILL_HINT}", file=sys.stderr)
    print("  Emergency bypass: AUDIT_GATE=off git commit ...", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
