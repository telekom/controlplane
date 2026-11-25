# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""The exact text a judge reads for one work unit, and how large it may be.

Rendering lives here rather than in the viewer so the packer can measure a
unit before it exists: a unit is only useful if the judge receives all of it.
"""

from __future__ import annotations

from .rules import Rule

#: Upper bound for one rendered work unit, in UTF-8 bytes. Agent harnesses cap
#: what a tool call may return - the Copilot CLI moves output beyond ~20 KB into
#: a temp file the judge is not allowed to read, so it answers nothing. The
#: margin absorbs the dispatch prompt and harness framing.
MAX_UNIT_BYTES = 16_000

#: Sections that are re-rendered with more structure further down, or that
#: only concern the engine. Repeating them wastes context and invites the
#: agent to re-run the detector in its head.
MACHINE_SECTIONS = ("## detect", "## checks", "## examples")

#: Room reserved for a candidate's header line and detector detail.
_CANDIDATE_SLACK = 512


def rule_prompt(rule: Rule) -> dict:
    return {
        "id": rule.id,
        "severity": rule.severity,
        "description": rule.description,
        "body": rule.body,
        "context_lines": rule.context_lines,
        "report_radius": rule.report_radius if rule.report_radius is not None else rule.context_lines,
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


def prose(body: str) -> str:
    kept = []
    for line in body.splitlines():
        if line.strip().lower() in MACHINE_SECTIONS:
            break
        kept.append(line)
    return "\n".join(kept).strip()


def render_unit(unit_id: str, rule: dict, items: list[dict]) -> str:
    """Render one work unit exactly as the viewer prints it."""
    out = [
        f"# Work unit {unit_id}",
        f"# Rule {rule['id']} [{rule['severity']}] - {rule['description']}",
        "",
        prose(rule["body"]),
        "",
    ]
    for kind in ("violation", "allowed"):
        for example in rule["examples"][kind]:
            out += [f"## Example: {kind}  (from {example['source']})", example["code"], ""]

    by_candidate: dict[str, list[dict]] = {}
    for item in items:
        by_candidate.setdefault(item["candidate_id"], []).append(item)

    for index, (_, group) in enumerate(sorted(by_candidate.items()), start=1):
        head = group[0]
        out.append(f"## Candidate {index}: {head['file']}:{head['line']}")
        if head["detail"]:
            out.append(f"({head['detail']})")
        out += ["", "### Context ('>' marks lines this change touched)", head["context"], ""]
        out.append("### Questions")
        for item in sorted(group, key=lambda i: i["check_id"]):
            out += [f"- item_id={item['item_id']}  [{item['check_id']}]", f"  {item['question']}"]
        out.append("")

    out.append(f"# Answer all {len(items)} item(s). Return JSON only.")
    return "\n".join(out) + "\n"


def unit_bytes(unit_id: str, rule: dict, items: list[dict]) -> int:
    return len(render_unit(unit_id, rule, items).encode("utf-8"))


def context_budget(rule: Rule) -> int:
    """Bytes left for one candidate's context in a single-candidate unit."""
    prompt = rule_prompt(rule)
    items = [
        {
            "item_id": "0" * 16,
            "candidate_id": "",
            "check_id": check["id"],
            "question": check["question"],
            "file": "",
            "line": 0,
            "detail": "",
            "context": "",
        }
        for check in prompt["checks"]
    ]
    overhead = unit_bytes(f"{rule.id}#000", prompt, items) + _CANDIDATE_SLACK
    return max(MAX_UNIT_BYTES - overhead, 0)
