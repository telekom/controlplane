#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""The commit/push gate: prove an audit covers exactly what is being committed.

No model, no network, no credentials - just hashes. For every staged blob and
every rule that applies to it, a receipt must exist under the current rule
text. Edit a file after auditing and its blob changes, the receipt vanishes and
the gate blocks. That is the whole mechanism.

Complexity 0 rules are re-run here directly: they are cheap, and catching them before
asking for a full audit is a better experience.

Usage::

    audit_gate.py [--mode staged|range] [--fail-on LEVEL] [--config-root DIR]
"""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from auditlib import changeset, detect, rules as rulelib  # noqa: E402
from auditlib.context import AuditContext, ConfigError, add_argument, resolve  # noqa: E402
from auditlib.context import config_hash, snapshot_config  # noqa: E402
from auditlib.judges import JudgeError  # noqa: E402
from auditlib.report import SEVERITY_RANK  # noqa: E402
from auditlib.report import item_id  # noqa: E402
from auditlib.store import (  # noqa: E402
    ReceiptStore,
    audited_model,
    receipt_key,
    Waivers,
)

SKILL_HINT = "Run the code-auditor skill, then commit again."


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=("staged", "range"), default="staged")
    comparison = parser.add_mutually_exclusive_group()
    comparison.add_argument("--base", help="exact committed comparison base (range mode)")
    comparison.add_argument("--target", help="target branch/ref; compare from its merge base")
    parser.add_argument(
        "--revision", default=os.environ.get("PRE_COMMIT_TO_REF") or "HEAD",
        help="committed tip (range mode; defaults to the pushed ref or HEAD)",
    )
    parser.add_argument(
        "--fail-on", choices=("LOW", "HIGH", "CRITICAL"), default="CRITICAL"
    )
    parser.add_argument("paths", nargs="*", help="ignored; accepted for pre-commit")
    add_argument(parser)
    args = parser.parse_args()
    if args.mode != "range" and (args.base or args.target):
        parser.error("--base and --target require --mode range")

    if os.environ.get("AUDIT_GATE", "").lower() in ("off", "0", "false"):
        print("audit-gate: disabled via AUDIT_GATE")
        return 0

    try:
        context = resolve(args.config_root)
    except ConfigError as exc:
        print(f"audit-gate: {exc}", file=sys.stderr)
        return 2
    try:
        if args.mode == "range":
            args.revision = changeset.commit(context.repo_root, args.revision)
            source_ref = os.environ.get("PRE_COMMIT_LOCAL_BRANCH")
            branch = None
            if source_ref is not None:
                # Tags and detached pushes have no branch-specific target.
                branch = (
                    source_ref.removeprefix("refs/heads/")
                    if source_ref.startswith("refs/heads/") else ""
                )
            args.base = (
                changeset.commit(context.repo_root, args.base) if args.base
                else changeset.merge_base(
                    context.repo_root, args.revision, args.target, branch=branch,
                )
            )
        with changeset.snapshot(context.repo_root, args.mode, args.revision) as tree:
            with snapshot_config(context, tree) as config:
                return check(context, args, tree, config)
    except changeset.GitError as exc:
        print(f"audit-gate: {exc}", file=sys.stderr)
        return 2


def check(
    context: AuditContext, args: argparse.Namespace, tree: Path, config: Path,
) -> int:
    root = context.repo_root
    try:
        rules = rulelib.load_index(
            config, context.audit_dir / "rules.index.json"
        )
    except rulelib.RuleError as exc:
        print(f"audit-gate: the rule corpus is invalid.\n{exc}", file=sys.stderr)
        return 1

    files = changeset.collect(
        root, mode=args.mode, base=args.base, revision=args.revision,
    )
    if not files:
        return 0

    files = changeset.freeze(root, tree, files)
    contents = {
        f.path: (tree / f.path).read_text(encoding="utf-8", errors="replace")
        for f in files
    }
    tree_hash = changeset.tree_hash(tree)
    configuration_hash = config_hash(config)
    waivers = Waivers(config)
    if waivers.errors:
        print("\n".join(waivers.errors), file=sys.stderr)
        return 2

    threshold = SEVERITY_RANK[args.fail_on]
    try:
        model = audited_model(context.audit_dir, config)
    except JudgeError as exc:
        print(f"audit-gate: cannot tell which judge to expect.\n{exc}", file=sys.stderr)
        return 2
    receipts = ReceiptStore(context.audit_dir)

    complexity0: list[str] = []
    missing: dict[str, set[str]] = {}
    blocked: list[str] = []

    for rule in rules:
        scoped = detect.applicable_files(rule, files)
        if not scoped:
            continue

        if rule.complexity == 0:
            try:
                for candidate in detect.run(rule, files, tree, contents):
                    ident = item_id(
                        rule.id, "violation", candidate.file, candidate.occurrence_anchor
                    )
                    if waivers.get(ident):
                        continue
                    if SEVERITY_RANK[rule.severity] >= threshold:
                        complexity0.append(
                            f"[{rule.severity}] {rule.id}  {candidate.file}:"
                            f"{candidate.line}  {candidate.detail or candidate.anchor}"
                        )
            except detect.ToolUnavailable as exc:
                print(f"audit-gate: {exc}", file=sys.stderr)
                return 2
            continue

        for file in scoped:
            receipt = receipts.get(receipt_key(
                file.blob_sha, rule.content_hash, model, file.path,
                file.touched_lines, tree_hash, configuration_hash,
            ))
            if receipt is None:
                missing.setdefault(file.path, set()).add(rule.id)
            elif receipt.get("verdict") not in ("PASS", "FAIL", "UNCLEAR"):
                print(
                    f"audit-gate: invalid receipt for {file.path}: {receipt}",
                    file=sys.stderr,
                )
                return 2
            elif (
                receipt["verdict"] in ("FAIL", "UNCLEAR")
                and SEVERITY_RANK[rule.severity] >= threshold
            ):
                blocked.append(
                    f"[{rule.severity}] {rule.id}  {file.path}  "
                    f"verdict={receipt['verdict']}"
                )

    if not complexity0 and not missing and not blocked:
        print(f"audit-gate: {len(files)} file(s) covered by a current audit.")
        return 0

    print("audit-gate: commit blocked.\n", file=sys.stderr)
    if complexity0:
        print("  Deterministic rule violations:", file=sys.stderr)
        for line in sorted(complexity0):
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
