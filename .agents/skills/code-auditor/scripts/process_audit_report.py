#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Validate the agent's verdicts, then settle the audit.

Nothing an agent writes is trusted until it survives this script:

* the verdict must be one of the allowed values
* every pending item must have been answered - no partial audits
* every quote must still be present in the file it cites, at or near the line
  it cites, which makes an invented finding mechanically unable to get through

Only then are receipts and cache entries written.

Usage::

    process_audit_report.py [--report PATH] [--verdicts PATH] [--fail-on LEVEL]
                            [--config-root DIR]
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from auditlib import SCHEMA_VERSION, VERDICTS, changeset, detect, rules as rulelib  # noqa: E402
from auditlib.context import AuditContext, ConfigError, add_argument, resolve  # noqa: E402
from auditlib.report import SEVERITY_RANK, worst  # noqa: E402
from auditlib.store import (  # noqa: E402
    ReceiptStore,
    VerdictCache,
    receipt_key,
    verdict_key,
)

ANCHOR_WINDOW = 10


class RejectedReport(Exception):
    """The agent output does not satisfy the contract and must be redone."""


def _normalize(text: str) -> str:
    return " ".join(text.split())


def _anchor(root: Path, path: str, line: int, quote: str, blob: str) -> int:
    """Return the real line the quote sits on, or raise if it is not there."""
    target = _normalize(quote)
    if not target:
        raise RejectedReport(f"{path}:{line}: evidence quote is empty")

    try:
        lines = changeset.git(root, "cat-file", "-p", blob).splitlines()
    except changeset.GitError as exc:
        raise RejectedReport(f"{path}: cannot read cited file: {exc}") from exc

    start = max(1, line - ANCHOR_WINDOW)
    end = min(len(lines), line + ANCHOR_WINDOW)
    for number in sorted(range(start, end + 1), key=lambda n: abs(n - line)):
        if target in _normalize(lines[number - 1]):
            return number

    # Multi-line quotes are anchored against the joined window instead.
    window = _normalize("\n".join(lines[start - 1 : end]))
    if target in window:
        return line

    raise RejectedReport(
        f"{path}:{line}: quoted evidence not found in the file "
        f"(searched lines {start}-{end}). Quote the source verbatim."
    )


def _in_scope(report_scope: str, changed_lines: set[int], line: int) -> bool:
    """Whether a finding lands on work this change is responsible for.

    A rule reads whole files so it can judge them in context, but by default
    it may only *report* on lines the change touched. Inheriting a file's
    backlog because you edited one line in it is how linters get switched off.
    Rules that genuinely need document-level judgement opt in with
    ``report_scope: file``.
    """
    if report_scope == "file":
        return True
    return line in changed_lines


def merge(root: Path, report: dict, verdicts: dict) -> list[dict]:
    if report.get("schema_version") != SCHEMA_VERSION:
        raise RejectedReport(
            f"report schema_version {report.get('schema_version')} != {SCHEMA_VERSION}"
        )

    answers: dict[str, dict] = {}
    for entry in verdicts.get("verdicts", []):
        ident = str(entry.get("item_id", "")).strip()
        if not ident:
            raise RejectedReport("a verdict entry has no item_id")
        if ident in answers:
            raise RejectedReport(f"item {ident} answered more than once")
        answers[ident] = entry

    items = report["items"]
    by_id = {i["item_id"]: i for i in items}

    unknown = sorted(set(answers) - set(by_id))
    if unknown:
        raise RejectedReport(
            f"verdicts reference unknown item(s): {', '.join(unknown[:5])}"
        )

    representatives: dict[str, dict] = {}
    changed = {
        f["path"]: set(f.get("changed_lines", f.get("added_lines", [])))
        for f in report["scope"]["files"]
    }
    blobs = {f["path"]: f["blob_sha"] for f in report["scope"]["files"]}
    prompts = report.get("rule_prompts", {})
    report_scopes = {r["id"]: r.get("report_scope", "changed") for r in report["rules"]}

    for item in items:
        if item["status"] == "resolved" and item["source"] != "cluster":
            if item["source"] == "cache":
                evidence = item.get("evidence") or {}
                line = evidence.get("line", item["line"])
                if item["verdict"] != "PASS":
                    line = _anchor(
                        root, item["file"], line, evidence.get("quote", ""),
                        blobs[item["file"]],
                    )
                item["in_scope"] = _finding_in_scope(
                    item, line, report_scopes, changed, prompts,
                )
            representatives.setdefault(item["cluster_id"], item)
            continue
        if item["source"] == "cluster":
            continue

        answer = answers.get(item["item_id"])
        if answer is None:
            raise RejectedReport(
                f"item {item['item_id']} ({item['rule_id']}/{item['check_id']} "
                f"at {item['file']}:{item['line']}) was not answered"
            )

        verdict = str(answer.get("verdict", "")).strip().upper()
        if verdict not in VERDICTS:
            raise RejectedReport(
                f"item {item['item_id']}: verdict '{verdict}' is not one of {VERDICTS}"
            )

        evidence = answer.get("evidence") or {}
        reason = str(evidence.get("reason", "")).strip()
        if not reason:
            raise RejectedReport(f"item {item['item_id']}: evidence needs a 'reason'")

        if verdict != "PASS":
            quote = str(evidence.get("quote", ""))
            line = int(evidence.get("line", item["line"]) or item["line"])
            resolved = _anchor(root, item["file"], line, quote, blobs[item["file"]])
            evidence = {
                "file": item["file"],
                "line": resolved,
                "quote": quote,
                "reason": reason,
            }
        else:
            evidence = {"file": item["file"], "line": item["line"], "reason": reason}

        item["verdict"] = verdict
        item["evidence"] = evidence
        item["status"] = "resolved"
        item["source"] = "agent"
        item["in_scope"] = _finding_in_scope(
            item, evidence.get("line", item["line"]), report_scopes, changed, prompts,
        )
        representatives.setdefault(item["cluster_id"], item)

    for item in items:
        if item["source"] != "cluster":
            continue
        rep = representatives.get(item["cluster_id"])
        if rep is None or rep.get("verdict") is None:
            raise RejectedReport(
                f"item {item['item_id']}: cluster {item['cluster_id']} has no "
                f"answered representative"
            )
        item["verdict"] = rep["verdict"]
        item["evidence"] = {
            "file": item["file"],
            "line": item["line"],
            "quote": item["anchor"],
            "reason": f"identical to {rep['file']}:{rep['line']} - {rep['evidence']['reason']}",
        }
        item["status"] = "resolved"
        item["in_scope"] = _in_scope(
            report_scopes.get(item["rule_id"], "changed"),
            changed.get(item["file"], set()),
            item["line"],
        )

    return items


def _finding_in_scope(
    item: dict, line: int, scopes: dict, changed: dict, prompts: dict,
) -> bool:
    lines = changed.get(item["file"], set())
    if _in_scope(scopes.get(item["rule_id"], "changed"), lines, line):
        return True
    radius = prompts.get(item["rule_id"], {}).get("context_lines", 0)
    return any(abs(line - number) <= radius for number in lines)


def persist(directory: Path, report: dict, items: list[dict]) -> None:
    if report.get("errors"):
        raise RejectedReport("cannot persist a report containing detector errors")
    model = report.get("model", "unknown")
    hashes = {r["id"]: r["content_hash"] for r in report["rules"]}
    cache = VerdictCache(directory)
    receipts = ReceiptStore(directory)

    for item in items:
        if item["source"] == "agent":
            cache.put(
                verdict_key(
                    hashes[item["rule_id"]],
                    detect.judgment_key(
                        item["file"], _normalize(item["anchor"]),
                        item["context"], item.get("detail", ""),
                    ) + item["check_id"],
                    model,
                    report["config_hash"],
                ),
                item["verdict"],
                item["evidence"],
                item["rule_id"],
            )

    blobs = {f["path"]: f["blob_sha"] for f in report["scope"]["files"]}
    scopes = {
        f["path"]: tuple(f.get("changed_lines", f["added_lines"]))
        for f in report["scope"]["files"]
    }
    covered: dict[tuple[str, str], list[str]] = {}
    for item in items:
        # Advisory findings must not latch the gate shut: the change is not
        # responsible for them and no edit here would clear them.
        if not item.get("in_scope", True):
            continue
        covered.setdefault((item["file"], item["rule_id"]), []).append(item["verdict"])

    severities = {r["id"]: r["severity"] for r in report["rules"]}
    for rule in report["rules"]:
        for path, blob in blobs.items():
            if not rulelib.Matcher(
                rule["applies_to"], rule["exclude"]
            ).matches(path):
                continue
            verdicts = covered.get((path, rule["id"]))
            # A rule with no candidates for a file still covers that file.
            outcome = worst(verdicts) if verdicts else "PASS"
            if rule["outcome"] == "ERROR":
                outcome = "ERROR"
            receipts.put(
                receipt_key(
                    blob, rule["content_hash"], model, path,
                    scopes[path], report["tree_hash"], report["config_hash"],
                ),
                rule["id"],
                path,
                outcome,
                severities[rule["id"]],
            )

    cache.save()
    receipts.save()


def context_mismatch(report: dict, context: AuditContext) -> str | None:
    """Explain why the report belongs to another repository or config root.

    Receipts and cache entries are only valid for the rules and the files the
    report was built from; settling it elsewhere would vouch for neither.
    Reports written before the context was recorded are accepted only without
    a shared config root; with one, a contextless report cannot be traced to
    this repository.
    """
    recorded = report.get("context")
    if recorded is None and context.shared:
        return (
            f"the report records no context, but this run resolves to "
            f"{context.describe()}; re-run create_audit_report.py with the same "
            "--config-root"
        )
    if recorded is None or recorded == context.describe():
        return None
    return (
        f"the report was created for {recorded}, but this run resolves to "
        f"{context.describe()}; pass the same --config-root as create_audit_report.py"
    )


def summarise(items: list[dict], fail_on: str) -> tuple[list[dict], list[dict], str]:
    threshold = SEVERITY_RANK[fail_on]
    hits = [
        i
        for i in items
        if i["verdict"] in ("FAIL", "UNCLEAR")
        and SEVERITY_RANK[i["severity"]] >= threshold
    ]
    order = lambda i: (
        -SEVERITY_RANK[i["severity"]],
        i["rule_id"],
        i["file"],
        i["line"],
    )
    findings = sorted((i for i in hits if i.get("in_scope", True)), key=order)
    advisory = sorted((i for i in hits if not i.get("in_scope", True)), key=order)
    scoped = [i["verdict"] for i in items if i.get("in_scope", True)]
    return findings, advisory, worst(scoped) if scoped else "PASS"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--report", type=Path, default=None)
    parser.add_argument("--verdicts", type=Path, default=None)
    parser.add_argument(
        "--fail-on", choices=("LOW", "HIGH", "CRITICAL"), default="HIGH"
    )
    parser.add_argument("--json", action="store_true")
    add_argument(parser)
    args = parser.parse_args()

    try:
        context = resolve(args.config_root)
    except ConfigError as exc:
        print(f"Cannot settle the audit: {exc}", file=sys.stderr)
        return 2
    root = context.repo_root
    report_path = args.report or (context.audit_dir / "report.json")
    verdicts_path = args.verdicts or (context.audit_dir / "verdicts.json")

    if not report_path.is_file():
        print(
            f"No audit report at {report_path}. Run create_audit_report.py first.",
            file=sys.stderr,
        )
        return 2

    report = json.loads(report_path.read_text(encoding="utf-8"))
    if report.get("errors"):
        if args.json:
            print(json.dumps({"overall": "ERROR", "errors": report["errors"]}))
        else:
            for error in report["errors"]:
                print(f"Audit rejected: {error}", file=sys.stderr)
        return 2
    mismatch = context_mismatch(report, context)
    if mismatch:
        print(f"Audit rejected: {mismatch}", file=sys.stderr)
        return 2
    verdicts = (
        json.loads(verdicts_path.read_text(encoding="utf-8"))
        if verdicts_path.is_file()
        else {"verdicts": []}
    )

    try:
        items = merge(root, report, verdicts)
    except RejectedReport as exc:
        print(f"Audit rejected: {exc}", file=sys.stderr)
        return 2

    persist(context.audit_dir, report, items)
    report["items"] = items
    report["settled"] = True
    report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")

    findings, advisory, overall = summarise(items, args.fail_on)

    if args.json:
        print(
            json.dumps(
                {"overall": overall, "findings": findings, "advisory": advisory},
                indent=2,
            )
        )
        return 1 if findings else 0

    stats = report["stats"]
    print(
        f"Audited {stats['files']} file(s) against {stats['rules_total']} rule(s): "
        f"{stats['items_total']} item(s) settled."
    )
    if report.get("errors"):
        for error in report["errors"]:
            print(f"  ERROR {error}")

    def render(entries: list[dict]) -> None:
        for entry in entries:
            evidence = entry["evidence"] or {}
            print(f"  [{entry['severity']}] {entry['rule_id']}/{entry['check_id']}")
            print(f"    {entry['file']}:{evidence.get('line', entry['line'])}")
            quote = evidence.get("quote")
            if quote:
                print(f"    > {quote.strip()}")
            print(f"    {evidence.get('reason', '')}")
            print(f"    waive with: {entry['item_id']}")
            print()

    if findings:
        print(f"\n{len(findings)} finding(s):\n")
        render(findings)
    else:
        print("No findings at or above " + args.fail_on + ". Clean.")

    if advisory:
        print(
            f"{len(advisory)} advisory note(s) outside this change "
            f"(not blocking, not your regression):\n"
        )
        render(advisory)

    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main())
