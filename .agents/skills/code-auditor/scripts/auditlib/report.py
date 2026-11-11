# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""The audit report: schema, stable item identity and work-unit batching."""

from __future__ import annotations

import hashlib
from dataclasses import asdict, dataclass, field

from . import SCHEMA_VERSION, SEVERITIES

#: Work units are packed by *candidate*, because every check of one candidate
#: shares the same frozen context. Packing by item instead would re-send that
#: context once per check and mix unrelated files into a single prompt.
CANDIDATES_PER_UNIT = {"CRITICAL": 2, "HIGH": 4, "LOW": 10}

#: Second cap, on the resulting question count. Smaller batches cost more and
#: vary less, so the dial is set by severity: be precise where a wrong answer
#: is expensive. A rule with many checks hits this cap before the other one.
MAX_ITEMS_PER_UNIT = {"CRITICAL": 8, "HIGH": 20, "LOW": 40}

SEVERITY_RANK = {name: rank for rank, name in enumerate(SEVERITIES)}


def item_id(rule_id: str, check_slug: str, path: str, occurrence_anchor: str) -> str:
    digest = hashlib.sha256()
    for part in (rule_id, check_slug, path, occurrence_anchor):
        digest.update(part.encode("utf-8"))
        digest.update(b"\0")
    return digest.hexdigest()[:16]


def candidate_id(rule_id: str, path: str, line: int, normalized_anchor: str) -> str:
    digest = hashlib.sha256()
    for part in (rule_id, path, str(line), normalized_anchor):
        digest.update(part.encode("utf-8"))
        digest.update(b"\0")
    return digest.hexdigest()[:16]


@dataclass
class Item:
    item_id: str
    candidate_id: str
    rule_id: str
    check_id: str
    question: str
    severity: str
    file: str
    line: int
    anchor: str
    cluster_id: str
    status: str = "pending"
    source: str = "detector"
    in_scope: bool = True
    verdict: str | None = None
    evidence: dict | None = None
    detail: str = ""
    context: str = ""


@dataclass
class WorkUnit:
    unit_id: str
    rule_id: str
    severity: str
    candidates: int = 0
    item_ids: list[str] = field(default_factory=list)


def build_work_units(items: list[Item]) -> list[WorkUnit]:
    """Group pending items rule-major and candidate-major, then pack.

    Rule-major keeps one rule's criteria per prompt (no cross-rule
    contamination) and amortises the rule text. Candidate-major keeps every
    check about one snippet together, so its frozen context is sent once.

    Packing order is (file, line, candidate id) and both caps are fixed, so
    the same change always produces byte-identical prompts.
    """
    pending = [i for i in items if i.status == "pending"]
    grouped: dict[str, dict[str, list[Item]]] = {}
    for item in pending:
        grouped.setdefault(item.rule_id, {}).setdefault(item.candidate_id, []).append(item)

    units: list[WorkUnit] = []
    for rule_id in sorted(grouped):
        candidates = grouped[rule_id]
        order = sorted(
            candidates,
            key=lambda cid: (candidates[cid][0].file, candidates[cid][0].line, cid),
        )
        severity = candidates[order[0]][0].severity
        max_candidates = CANDIDATES_PER_UNIT.get(severity, 4)
        max_items = MAX_ITEMS_PER_UNIT.get(severity, 20)

        bucket: list[Item] = []
        bucket_candidates = 0
        index = 0

        def flush():
            nonlocal bucket, bucket_candidates, index
            if not bucket:
                return
            units.append(
                WorkUnit(
                    unit_id=f"{rule_id}#{index:03d}",
                    rule_id=rule_id,
                    severity=severity,
                    candidates=bucket_candidates,
                    item_ids=[i.item_id for i in bucket],
                )
            )
            index += 1
            bucket, bucket_candidates = [], 0

        for cid in order:
            group = sorted(candidates[cid], key=lambda i: i.check_id)
            too_many_candidates = bucket_candidates + 1 > max_candidates
            too_many_items = len(bucket) + len(group) > max_items
            if bucket and (too_many_candidates or too_many_items):
                flush()
            bucket.extend(group)
            bucket_candidates += 1
        flush()

    return units


def worst(verdicts: list[str]) -> str:
    if "FAIL" in verdicts:
        return "FAIL"
    if "UNCLEAR" in verdicts:
        return "UNCLEAR"
    return "PASS"


def render(
    *,
    base: str,
    mode: str,
    model: str,
    rules_index_hash: str,
    files: list[dict],
    rules_summary: list[dict],
    items: list[Item],
    units: list[WorkUnit],
    errors: list[str],
) -> dict:
    return {
        "schema_version": SCHEMA_VERSION,
        "mode": mode,
        "base": base,
        "model": model,
        "rules_index_hash": rules_index_hash,
        "scope": {"files": files},
        "rules": rules_summary,
        "stats": {
            "files": len(files),
            "rules_total": len(rules_summary),
            "rules_with_candidates": sum(
                1 for r in rules_summary if r["candidates"] > 0
            ),
            "items_total": len(items),
            "items_pending": sum(1 for i in items if i.status == "pending"),
            "items_resolved": sum(1 for i in items if i.status == "resolved"),
            "work_units": len(units),
        },
        "errors": errors,
        "work_units": [asdict(u) for u in units],
        "items": [asdict(i) for i in items],
    }
