#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Measure whether a rule actually works, and whether it works repeatably.

A rule is a piece of software and deserves the same treatment as one. Its
examples already carry provenance, so they are fixtures with a known expected
verdict: a ``### Violation`` must be caught, an ``### Allowed`` must not.

Two things get measured:

**Correctness** - does the rule classify its own fixtures right.
**Flake rate** - run the same fixture N times and count the cells where the
answers disagree. You cannot make a model deterministic, but you can refuse to
merge a rule that is not.

Leave-one-out matters: when a fixture is judged, that example is **removed**
from the prompt. Otherwise the agent is shown the answer and the test measures
nothing.

Usage::

    selftest_rules.py --static                      # no model, runs in CI
    selftest_rules.py --plan --samples 3            # build fixture work units
    selftest_rules.py --score --max-flake 0.05      # grade the answers
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from auditlib import SCHEMA_VERSION, VERDICTS, changeset, detect  # noqa: E402
from auditlib import rules as rulelib  # noqa: E402
from auditlib.judges import JudgeError  # noqa: E402
from auditlib.store import audit_dir, model_id  # noqa: E402

YIELD_SAMPLE_FILES = 300
DEFAULT_MAX_FLAKE = 0.05


def selftest_dir(root: Path) -> Path:
    return audit_dir(root) / "selftest"


@dataclass
class Fixture:
    rule: rulelib.Rule
    kind: str
    index: int
    example: rulelib.Example
    context: str
    anchor_line: int
    path: str
    expected_check: str = ""
    items: dict = field(default_factory=dict)

    @property
    def fixture_id(self) -> str:
        return f"{self.rule.id}@{self.kind}{self.index}"

    @property
    def expects_fail(self) -> bool:
        return self.kind == "violation"


def _git_show(root: Path, rev: str, path: str) -> list[str] | None:
    result = subprocess.run(
        ["git", "show", f"{rev}:{path}"], cwd=root, capture_output=True, text=True
    )
    if result.returncode != 0:
        return None
    return result.stdout.splitlines()


def build_fixtures(root: Path, rules: list[rulelib.Rule]) -> tuple[list[Fixture], list[str]]:
    """Turn every example into a fixture with real surrounding context."""
    fixtures, skipped = [], []
    for rule in rules:
        if not rule.checks:
            skipped.append(f"{rule.id}: complexity 0, decided by the detector - nothing to judge")
            continue
        for kind in ("violation", "allowed"):
            for index, example in enumerate(rule.examples.get(kind, ())):
                if example.provenance:
                    lines = _git_show(root, example.provenance.rev, example.provenance.path)
                    if lines is None:
                        skipped.append(
                            f"{rule.id}: {kind} {index} pinned at "
                            f"{example.provenance.rev} is not in this clone"
                        )
                        continue
                    span = len(
                        [line for line in example.code.splitlines() if line.strip()]
                    )
                    anchor = example.provenance.line
                    marked = frozenset(range(anchor, anchor + max(span, 1)))
                    context = detect.render_context(
                        lines, anchor, rule.context_lines, marked
                    )
                    path = example.provenance.path
                else:
                    lines = example.code.splitlines()
                    anchor = 1
                    context = detect.render_context(
                        lines, 1, len(lines) + 1, frozenset(range(1, len(lines) + 1))
                    )
                    path = f"{rule.id}#synthetic-{index}"

                fixtures.append(
                    Fixture(
                        rule=rule,
                        kind=kind,
                        index=index,
                        example=example,
                        context=context,
                        anchor_line=anchor,
                        path=path,
                    )
                )
    return fixtures, skipped


def _held_out_prompt(fixture: Fixture) -> dict:
    """The rule as the agent should see it: without the fixture's own example.

    Leaving the example in would hand over the answer. This is the difference
    between a test and a demonstration.
    """
    rule = fixture.rule
    examples = {}
    for kind in ("violation", "allowed"):
        kept = []
        for index, example in enumerate(rule.examples.get(kind, ())):
            if kind == fixture.kind and index == fixture.index:
                continue
            kept.append(
                {
                    "code": example.code,
                    "source": str(example.provenance) if example.provenance else "synthetic",
                }
            )
        examples[kind] = kept
    return {
        "id": fixture.fixture_id,
        "severity": rule.severity,
        "description": rule.description,
        "body": rule.body,
        "checks": [{"id": c.slug, "question": c.question} for c in rule.checks],
        "examples": examples,
    }


def _item_id(fixture_id: str, sample: int, check: str) -> str:
    digest = hashlib.sha256()
    for part in (fixture_id, str(sample), check):
        digest.update(part.encode("utf-8"))
        digest.update(b"\0")
    return digest.hexdigest()[:16]


def plan(root: Path, rules: list[rulelib.Rule], samples: int) -> dict:
    fixtures, skipped = build_fixtures(root, rules)
    items, units, prompts = [], [], {}

    for fixture in fixtures:
        prompts[fixture.fixture_id] = _held_out_prompt(fixture)
        candidate = f"{fixture.fixture_id}:cand"
        for sample in range(samples):
            unit_items = []
            for check in fixture.rule.checks:
                ident = _item_id(fixture.fixture_id, sample, check.slug)
                items.append(
                    {
                        "item_id": ident,
                        "candidate_id": f"{candidate}:s{sample}",
                        "rule_id": fixture.fixture_id,
                        "source_rule_id": fixture.rule.id,
                        "check_id": check.slug,
                        "question": check.question,
                        "severity": fixture.rule.severity,
                        "file": fixture.path,
                        "line": fixture.anchor_line,
                        "anchor": fixture.path,
                        "cluster_id": f"{candidate}:s{sample}:{check.slug}",
                        "status": "pending",
                        "source": "fixture",
                        "verdict": None,
                        "evidence": None,
                        "detail": (
                            f"fixture {fixture.kind} #{fixture.index}, sample {sample}; "
                            f"'>' marks the example under test"
                        ),
                        "context": fixture.context,
                        "sample": sample,
                        "expects_fail": fixture.expects_fail,
                    }
                )
                unit_items.append(ident)
            units.append(
                {
                    "unit_id": f"{fixture.fixture_id}#s{sample:02d}",
                    "rule_id": fixture.fixture_id,
                    "severity": fixture.rule.severity,
                    "candidates": 1,
                    "item_ids": unit_items,
                }
            )

    return {
        "schema_version": SCHEMA_VERSION,
        "mode": "selftest",
        "base": "fixtures",
        "model": model_id(root),
        "rules_index_hash": rulelib.index_hash(rules),
        "samples": samples,
        "scope": {"files": []},
        "rules": [
            {
                "id": r.id,
                "severity": r.severity,
                "complexity": r.complexity,
                "report_scope": r.report_scope,
                "content_hash": r.content_hash,
                "files_in_scope": 0,
                "candidates": 0,
                "outcome": "PENDING",
                "reason": "",
            }
            for r in rules
        ],
        "stats": {
            "fixtures": len(fixtures),
            "samples": samples,
            "items_total": len(items),
            "items_pending": len(items),
            "items_resolved": 0,
            "work_units": len(units),
            "files": 0,
            "rules_total": len(rules),
            "rules_with_candidates": len({i["source_rule_id"] for i in items}),
        },
        "errors": [],
        "skipped": skipped,
        "work_units": units,
        "items": items,
        "rule_prompts": prompts,
    }


def static(root: Path, rules: list[rulelib.Rule]) -> tuple[list[dict], list[str]]:
    """Everything measurable without a model: detector agreement and yield."""
    tracked = changeset.git(root, "ls-files").splitlines()
    rows, problems = [], []

    for rule in rules:
        matcher = rulelib.Matcher(rule.applies_to, rule.exclude)
        scoped = sorted(p for p in tracked if matcher.matches(p))
        sample = scoped[:: max(1, len(scoped) // YIELD_SAMPLE_FILES)][:YIELD_SAMPLE_FILES]

        # Candidates, not raw matches: a scope:file rule collapses a file's
        # matches into one. And questions, not candidates, is what a model is
        # actually billed for.
        # Count what a model would actually be asked: candidates from the
        # detector, times only the checks whose own trigger fires. The whole
        # file stands in for the context window, so this is an upper bound.
        candidates = None if rule.detector and rule.detector.kind == "tool" else 0
        questions = 0
        if candidates is not None:
            pattern = (
                re.compile(rule.detector.pattern) if rule.detector else None
            )
            for path in sample:
                try:
                    text = (root / path).read_text(encoding="utf-8")
                except (OSError, UnicodeDecodeError):
                    continue
                if pattern is None:
                    found = 1
                else:
                    matched = [ln for ln in text.splitlines() if pattern.search(ln)]
                    if not matched:
                        continue
                    found = 1 if rule.scope == "file" else len(matched)
                candidates += found
                questions += found * sum(
                    1 for c in rule.checks if c.applies_to_context(text)
                )

        cand_per_100 = (
            round(candidates / len(sample) * 100, 1)
            if candidates is not None and sample
            else None
        )
        questions_per_100 = (
            round(questions / len(sample) * 100, 1)
            if candidates is not None and sample
            else None
        )

        # Density predicts the cost of one change; exposure predicts the cost
        # of touching everything the rule applies to. A complexity 2 rule is always
        # dense by construction, so only exposure says anything about it.
        exposure = (
            round(questions_per_100 / 100 * len(scoped))
            if questions_per_100 is not None
            else None
        )

        violations = rule.examples.get("violation", ())
        sourced = sum(1 for g in rule.examples.values() for e in g if e.is_sourced)

        if rule.checks and not violations:
            problems.append(f"{rule.id}: no violation example, nothing to test against")
        if exposure is not None and exposure > 400:
            problems.append(
                f"{rule.id}: {exposure} model questions if every in-scope file were "
                f"touched; tighten the detector, narrow applies_to, or split the checks"
            )
        if (
            rule.complexity == 1
            and questions_per_100 is not None
            and questions_per_100 > 150
        ):
            problems.append(
                f"{rule.id}: {questions_per_100:.0f} questions per 100 files - the "
                f"detector is not selective enough for a complexity 1 rule"
            )

        untested, unreachable = [], []
        for check in rule.checks:
            if not check.when:
                continue
            if not any(
                check.applies_to_context(e.code)
                for group in rule.examples.values()
                for e in group
            ):
                untested.append(check.slug)
            if rule.detector and rule.detector.kind == "regex":
                detector = re.compile(rule.detector.pattern)
                probe = re.compile(check.when)
                for group in rule.examples.values():
                    for example in group:
                        for line in example.code.splitlines():
                            if probe.search(line) and not detector.search(line):
                                unreachable.append(check.slug)
                                break
        if unreachable:
            problems.append(
                f"{rule.id}: check(s) {sorted(set(unreachable))} can trigger on code the "
                f"rule detector does not match, so they would never be asked; widen "
                f"'## Detect'"
            )

        rows.append(
            {
                "id": rule.id,
                "complexity": rule.complexity,
                "checks": len(rule.checks),
                "files": len(scoped),
                "sampled": len(sample),
                "candidates_per_100": cand_per_100,
                "questions_per_100": questions_per_100,
                "exposure": exposure,
                "untested_checks": sorted(set(untested)),
                "fixtures": sum(len(g) for g in rule.examples.values()),
                "sourced": sourced,
            }
        )
    return rows, problems


def score(root: Path, plan_doc: dict, verdicts: dict, max_flake: float) -> tuple[dict, int]:
    answers = {}
    for entry in verdicts.get("verdicts", []):
        answers[str(entry.get("item_id", ""))] = entry

    by_fixture: dict[str, dict] = {}
    unanswered = []

    for item in plan_doc["items"]:
        answer = answers.get(item["item_id"])
        if answer is None:
            unanswered.append(item["item_id"])
            continue
        verdict = str(answer.get("verdict", "")).strip().upper()
        if verdict not in VERDICTS:
            unanswered.append(item["item_id"])
            continue
        entry = by_fixture.setdefault(
            item["rule_id"],
            {
                "source_rule": item["source_rule_id"],
                "expects_fail": item["expects_fail"],
                "samples": {},
                "cells": {},
            },
        )
        entry["samples"].setdefault(item["sample"], {})[item["check_id"]] = verdict
        entry["cells"].setdefault(item["check_id"], []).append(verdict)

    per_rule: dict[str, dict] = {}
    for fixture_id, data in sorted(by_fixture.items()):
        rule_id = data["source_rule"]
        bucket = per_rule.setdefault(
            rule_id, {"correct": 0, "total": 0, "flaky_cells": 0, "cells": 0, "detail": []}
        )

        outcomes = []
        for sample in sorted(data["samples"]):
            verdicts_for_sample = data["samples"][sample]
            caught = any(v == "FAIL" for v in verdicts_for_sample.values())
            ok = caught if data["expects_fail"] else not caught
            outcomes.append(ok)
            bucket["total"] += 1
            bucket["correct"] += 1 if ok else 0

        for check, values in data["cells"].items():
            bucket["cells"] += 1
            if len(set(values)) > 1:
                bucket["flaky_cells"] += 1
                bucket["detail"].append(
                    f"{fixture_id}/{check}: {' '.join(values)}"
                )

        if not all(outcomes):
            bucket["detail"].append(
                f"{fixture_id}: expected "
                f"{'FAIL' if data['expects_fail'] else 'PASS'}, got "
                f"{['caught' if o == data['expects_fail'] else 'missed' for o in outcomes]}"
            )

    failures = 0
    for rule_id, bucket in per_rule.items():
        bucket["accuracy"] = (
            bucket["correct"] / bucket["total"] if bucket["total"] else 0.0
        )
        bucket["flake_rate"] = (
            bucket["flaky_cells"] / bucket["cells"] if bucket["cells"] else 0.0
        )
        if bucket["accuracy"] < 1.0 or bucket["flake_rate"] > max_flake:
            failures += 1

    return (
        {"rules": per_rule, "unanswered": unanswered, "max_flake": max_flake},
        1 if (failures or unanswered) else 0,
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--static", action="store_true", help="no-model checks only")
    parser.add_argument("--plan", action="store_true", help="build fixture work units")
    parser.add_argument("--score", action="store_true", help="grade the answers")
    parser.add_argument("--samples", type=int, default=1)
    parser.add_argument("--rule", action="append", default=[], help="filter by rule id")
    parser.add_argument("--max-flake", type=float, default=DEFAULT_MAX_FLAKE)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()

    root = changeset.repo_root()
    try:
        rules = rulelib.load_rules(root)
    except rulelib.RuleError as exc:
        print(f"Cannot selftest: the rule corpus is invalid.\n{exc}", file=sys.stderr)
        return 2
    if args.rule:
        rules = [r for r in rules if r.id in set(args.rule)]
        if not rules:
            print(f"No rule matches {args.rule}", file=sys.stderr)
            return 2

    out = selftest_dir(root)

    if args.static:
        rows, problems = static(root, rules)
        if args.json:
            print(json.dumps({"rules": rows, "problems": problems}, indent=2))
        else:
            def cell(value):
                return "n/a" if value is None else f"{value}"

            print(
                f"{'rule':34} {'complexity':>10} {'checks':>6} {'files':>6} "
                f"{'cand/100':>9} {'quest/100':>10} {'exposure':>9} "
                f"{'fixtures':>9} {'sourced':>8}"
            )
            for row in rows:
                print(
                    f"{row['id']:34} {row['complexity']:>10} {row['checks']:>6} "
                    f"{row['files']:>6} {cell(row['candidates_per_100']):>9} "
                    f"{cell(row['questions_per_100']):>10} "
                    f"{cell(row['exposure']):>9} "
                    f"{row['fixtures']:>9} {row['sourced']:>8}"
                )
            for row in rows:
                if row["untested_checks"]:
                    print(
                        f"  note: {row['id']} has no example exercising "
                        f"{row['untested_checks']}"
                    )
            for problem in problems:
                print(f"  - {problem}", file=sys.stderr)
        return 1 if problems else 0

    if args.plan:
        try:
            document = plan(root, rules, args.samples)
        except JudgeError as exc:
            print(f"Cannot selftest: no judge to run on.\n{exc}", file=sys.stderr)
            return 2
        out.mkdir(parents=True, exist_ok=True)
        (out / "plan.json").write_text(json.dumps(document, indent=2) + "\n", "utf-8")
        verdicts_path = out / "verdicts.json"
        if not verdicts_path.exists():
            verdicts_path.write_text(json.dumps({"verdicts": []}, indent=2) + "\n", "utf-8")
        stats = document["stats"]
        print(f"Selftest plan: {out / 'plan.json'}")
        print(
            f"  {stats['fixtures']} fixture(s) x {args.samples} sample(s) = "
            f"{stats['items_total']} item(s) in {stats['work_units']} work unit(s)"
        )
        for note in document["skipped"]:
            print(f"  skipped: {note}")
        print("\nWork units:")
        for unit in document["work_units"]:
            print(f"  {unit['unit_id']}  ({len(unit['item_ids'])} item(s))")
        return 0

    if args.score:
        plan_path = out / "plan.json"
        if not plan_path.is_file():
            print(f"No plan at {plan_path}. Run --plan first.", file=sys.stderr)
            return 2
        plan_doc = json.loads(plan_path.read_text(encoding="utf-8"))
        verdicts = json.loads((out / "verdicts.json").read_text(encoding="utf-8"))
        result, code = score(root, plan_doc, verdicts, args.max_flake)
        (out / "results.json").write_text(json.dumps(result, indent=2) + "\n", "utf-8")

        if args.json:
            print(json.dumps(result, indent=2))
            return code

        print(f"{'rule':28} {'accuracy':>9} {'flake':>7} {'cells':>6}")
        for rule_id, bucket in sorted(result["rules"].items()):
            print(
                f"{rule_id:28} {bucket['correct']}/{bucket['total']:<7} "
                f"{bucket['flake_rate']:>6.0%} {bucket['cells']:>6}"
            )
        for rule_id, bucket in sorted(result["rules"].items()):
            for line in bucket["detail"]:
                print(f"  {rule_id}: {line}")
        if result["unanswered"]:
            print(
                f"\n{len(result['unanswered'])} item(s) unanswered - the selftest is "
                f"incomplete, not passing.",
                file=sys.stderr,
            )
        print(f"\nflake budget: {args.max_flake:.0%}")
        return code

    parser.print_help()
    return 2


if __name__ == "__main__":
    sys.exit(main())
