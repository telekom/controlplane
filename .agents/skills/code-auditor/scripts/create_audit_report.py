#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Generate the audit report for the current change.

This script does all of the deterministic work: resolve the changeset, filter
rules by glob, run detectors, settle complexity 0 rules outright, reuse cached
verdicts, cluster identical snippets and batch whatever is left into work
units. A model is only ever asked about items that survive all of that.

Usage::

    create_audit_report.py --mode local|staged|range|worktree
                           [--base TARGET] [--revision REF]
                           [--out PATH] [--config-root DIR]
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from auditlib import changeset, detect, report as reportlib, rules as rulelib  # noqa: E402
from auditlib import judges as judgelib  # noqa: E402
from auditlib.context import AuditContext, ConfigError, add_argument, resolve  # noqa: E402
from auditlib.context import config_hash, snapshot_config  # noqa: E402
from auditlib.judges import JudgeError  # noqa: E402
from auditlib.store import (  # noqa: E402
    VerdictCache,
    Waivers,
    verdict_key,
    write_state,
)


def _rule_prompt(rule: rulelib.Rule) -> dict:
    return {
        "id": rule.id,
        "severity": rule.severity,
        "description": rule.description,
        "body": rule.body,
        "context_lines": rule.context_lines,
        "checks": [{"id": c.slug, "question": c.question} for c in rule.checks],
        "examples": {
            kind: [
                {
                    "code": e.code,
                    "source": str(e.provenance) if e.provenance else "synthetic",
                }
                for e in rule.examples.get(kind, ())
            ]
            for kind in ("violation", "allowed")
        },
    }


def build(
    context: AuditContext, mode: str, *,
    base: str | None = None, revision: str = "HEAD", announce: bool = False,
) -> dict:
    root = context.repo_root
    tip = changeset.commit(root, revision)
    comparison_base = (
        changeset.commit(root, "HEAD") if mode in ("local", "staged")
        else changeset.merge_base(root, tip, target=base)
    )
    comparison = {
        "target": (
            base or changeset.target_branch(root)
            if mode in ("range", "worktree") else None
        ),
        "base": comparison_base,
        "revision": tip if mode == "range" else (
            "INDEX" if mode == "staged" else "WORKTREE"
        ),
    }
    if announce:
        target = f" (target {comparison['target']})" if comparison["target"] else ""
        print(
            f"Scope: {mode}{target}; "
            f"{comparison_base} -> {comparison['revision']}",
            flush=True,
        )
    with changeset.snapshot(root, mode, tip) as tree:
        with snapshot_config(context, tree) as config:
            return _build(context, mode, tree, comparison, tip, config)


def _build(
    context: AuditContext, mode: str, input_tree: Path,
    comparison: dict, revision: str, config: Path,
) -> dict:
    root = context.repo_root
    configuration_hash = config_hash(config)
    judge = judgelib.load(config).resolve()
    model = judge.pin
    rules = rulelib.load_index(
        config, context.audit_dir / "rules.index.json"
    )
    base = comparison["base"]
    files = changeset.collect(root, mode=mode, base=base, revision=revision)
    files = changeset.freeze(root, input_tree, files)

    contents = {
        f.path: (input_tree / f.path).read_text(encoding="utf-8", errors="replace")
        for f in files
    }

    cache = VerdictCache(context.audit_dir)
    waivers = Waivers(config)

    items: list[reportlib.Item] = []
    rules_summary: list[dict] = []
    errors: list[str] = list(waivers.errors)
    cluster_seen: dict[str, str] = {}

    for rule in rules:
        scoped = detect.applicable_files(rule, files)
        if not scoped:
            rules_summary.append(
                {
                    "id": rule.id,
                    "severity": rule.severity,
                    "complexity": rule.complexity,
                    "report_scope": rule.report_scope,
                    "content_hash": rule.content_hash,
                    "files_in_scope": 0,
                    "candidates": 0,
                    "outcome": "PASS",
                    "reason": "no changed file matches applies_to",
                }
            )
            continue

        try:
            candidates = detect.run(rule, files, input_tree, contents)
        except detect.ToolUnavailable as exc:
            errors.append(str(exc))
            rules_summary.append(
                {
                    "id": rule.id,
                    "severity": rule.severity,
                    "complexity": rule.complexity,
                    "report_scope": rule.report_scope,
                    "content_hash": rule.content_hash,
                    "files_in_scope": len(scoped),
                    "candidates": 0,
                    "outcome": "ERROR",
                    "reason": str(exc),
                }
            )
            continue

        if not candidates:
            rules_summary.append(
                {
                    "id": rule.id,
                    "severity": rule.severity,
                    "complexity": rule.complexity,
                    "report_scope": rule.report_scope,
                    "content_hash": rule.content_hash,
                    "files_in_scope": len(scoped),
                    "candidates": 0,
                    "outcome": "PASS",
                    "reason": "detector produced no candidates",
                }
            )
            continue

        checks = rule.checks or (rulelib.Check(slug="violation", question=""),)
        for candidate in candidates:
            cluster = candidate.cluster_key(rule)
            cand_id = reportlib.candidate_id(
                rule.id, candidate.file, candidate.line, candidate.normalized_anchor
            )
            for check in checks:
                ident = reportlib.item_id(
                    rule.id, check.slug, candidate.file, candidate.occurrence_anchor
                )
                item = reportlib.Item(
                    item_id=ident,
                    candidate_id=cand_id,
                    rule_id=rule.id,
                    check_id=check.slug,
                    question=check.question,
                    severity=rule.severity,
                    file=candidate.file,
                    line=candidate.line,
                    anchor=candidate.anchor,
                    cluster_id=f"{cluster}:{check.slug}",
                    detail=candidate.detail,
                    context=candidate.context,
                )

                waiver = waivers.get(ident)
                if waiver:
                    item.status = "resolved"
                    item.source = "waiver"
                    item.verdict = "PASS"
                    item.evidence = {"reason": f"waived: {waiver.reason}"}
                elif not check.applies_to_context(candidate.context):
                    # The check declares a trigger and nothing in the window
                    # matches it. Answer it here rather than paying a model to
                    # confirm the absence of a string - but answer it visibly,
                    # never as a silent gap.
                    item.status = "resolved"
                    item.source = "detector"
                    item.verdict = "PASS"
                    item.evidence = {
                        "file": candidate.file,
                        "line": candidate.line,
                        "reason": (
                            f"no '{check.slug}' trigger in the candidate window "
                            f"(when=/{check.when}/)"
                        ),
                    }
                elif rule.decides_without_llm:
                    item.status = "resolved"
                    item.source = "tool"
                    item.verdict = "FAIL"
                    item.evidence = {
                        "file": candidate.file,
                        "line": candidate.line,
                        "quote": candidate.anchor,
                        "reason": candidate.detail or rule.description,
                    }
                else:
                    key = verdict_key(
                        rule.content_hash, candidate.judgment_key + check.slug, model,
                        configuration_hash,
                    )
                    cached = cache.get(key)
                    if cached:
                        item.status = "resolved"
                        item.source = "cache"
                        item.verdict = cached["verdict"]
                        item.evidence = cached.get("evidence")
                    elif item.cluster_id in cluster_seen:
                        item.status = "resolved"
                        item.source = "cluster"
                        item.evidence = {"same_as": cluster_seen[item.cluster_id]}
                    else:
                        cluster_seen[item.cluster_id] = ident

                items.append(item)

        rules_summary.append(
            {
                "id": rule.id,
                "severity": rule.severity,
                "complexity": rule.complexity,
                "report_scope": rule.report_scope,
                "content_hash": rule.content_hash,
                "files_in_scope": len(scoped),
                "candidates": len(candidates),
                "outcome": "PENDING" if rule.complexity > 0 else "FAIL",
                "reason": "",
            }
        )

    items.sort(key=lambda i: (i.rule_id, i.file, i.line, i.check_id))
    units = reportlib.build_work_units(items)
    cache.save()
    by_id = {rule.id: rule for rule in rules}
    for summary in rules_summary:
        rule = by_id[summary["id"]]
        summary["applies_to"] = list(rule.applies_to)
        summary["exclude"] = list(rule.exclude)

    return reportlib.render(
        base=base,
        mode=mode,
        model=model,
        rules_index_hash=rulelib.index_hash(rules),
        files=[
            {
                "path": f.path,
                "blob_sha": f.blob_sha,
                "added_lines": list(f.added_lines),
                "changed_lines": list(f.touched_lines),
            }
            for f in files
        ],
        rules_summary=rules_summary,
        items=items,
        units=units,
        errors=errors,
    ) | {
        "judge": judge.complexity,
        "context": context.describe(),
        "comparison": comparison,
        "config_hash": configuration_hash,
        "tree_hash": changeset.tree_hash(input_tree),
        "rule_prompts": {r.id: _rule_prompt(r) for r in rules if r.complexity > 0},
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--mode", choices=("local", "staged", "range", "worktree"), required=True,
        help=(
            "local: uncommitted edits vs HEAD; staged: index vs HEAD; "
            "range: committed branch/PR changes; worktree: branch changes plus local edits"
        ),
    )
    parser.add_argument("--base", help="target branch/ref for range or worktree (uses merge base)")
    parser.add_argument("--revision", help="committed tip to review in range mode (default: HEAD)")
    parser.add_argument("--out", type=Path, default=None)
    parser.add_argument("--quiet", action="store_true")
    add_argument(parser)
    args = parser.parse_args()
    if args.base is not None and args.mode not in ("range", "worktree"):
        parser.error("--base is only valid with --mode range or worktree")
    if args.revision is not None and args.mode != "range":
        parser.error("--revision is only valid with --mode range")

    try:
        context = resolve(args.config_root)
        document = build(
            context, args.mode, base=args.base, revision=args.revision or "HEAD",
            announce=not args.quiet,
        )
    except ConfigError as exc:
        print(f"Cannot audit: {exc}", file=sys.stderr)
        return 2
    except rulelib.RuleError as exc:
        print(f"Cannot audit: the rule corpus is invalid.\n{exc}", file=sys.stderr)
        return 2
    except JudgeError as exc:
        print(f"Cannot audit: no judge to run on.\n{exc}", file=sys.stderr)
        return 2
    except changeset.GitError as exc:
        print(f"Cannot audit: {exc}", file=sys.stderr)
        return 2

    out = args.out or (context.audit_dir / "report.json")
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(document, indent=2) + "\n", encoding="utf-8")
    write_state(
        context.audit_dir, document["model"], document["rules_index_hash"],
        document["judge"],
    )

    verdicts_path = out.parent / "verdicts.json"
    verdicts_path.write_text(json.dumps({"verdicts": []}, indent=2) + "\n", "utf-8")

    if not args.quiet:
        stats = document["stats"]
        print(f"Audit report: {out}")
        print(
            f"  {stats['files']} file(s), {stats['rules_total']} rules, "
            f"{stats['rules_with_candidates']} with candidates"
        )
        print(
            f"  {stats['items_total']} item(s): {stats['items_resolved']} resolved "
            f"deterministically, {stats['items_pending']} need judgment"
        )
        print(f"  {stats['work_units']} work unit(s)")
    for error in document["errors"]:
        print(f"  ERROR {error}", file=sys.stderr)
    return 2 if document["errors"] else 0


if __name__ == "__main__":
    sys.exit(main())
