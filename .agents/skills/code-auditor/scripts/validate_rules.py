#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Static validation of the rule corpus.

Run by pre-commit so the ruleset cannot drift. Every check here is a check the
audit engine would otherwise have to make at runtime against an untrusted rule.

Usage::

    validate_rules.py [--json]
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import re  # noqa: E402

from auditlib import changeset, provenance, rules as rulelib  # noqa: E402
from auditlib.store import Waivers  # noqa: E402

MAX_CHECKS_PER_RULE = 8


def check_provenance(root, rules: list[rulelib.Rule]) -> tuple[list[str], list[str]]:
    """Re-anchor every sourced example against its pinned revision."""
    problems, skipped = [], []
    for rule in rules:
        for group in rule.examples.values():
            for example in group:
                if not example.is_sourced:
                    continue
                failure = provenance.verify(root, example.provenance, example.code)
                if failure is None:
                    continue
                if failure.startswith("UNAVAILABLE:"):
                    skipped.append(f"{rule.id}: {failure}")
                    continue
                problems.append(f"{rule.id}: {example.kind} example {failure}")
    return problems, skipped


def check_detector_agrees(rules: list[rulelib.Rule]) -> list[str]:
    """A regex detector that misses its own violation example is broken."""
    problems = []
    for rule in rules:
        if rule.detector is None or rule.detector.kind != "regex":
            continue
        pattern = re.compile(rule.detector.pattern, re.MULTILINE)
        for example in rule.examples.get("violation", ()):
            if not pattern.search(example.code):
                problems.append(
                    f"{rule.id}: the detector does not match its own violation "
                    f"example, so the rule can never fire on it"
                )
    return problems


def extra_checks(rules: list[rulelib.Rule]) -> list[str]:
    """Authoring-quality checks that go beyond 'is this parseable'."""
    problems = []
    for rule in rules:
        if len(rule.checks) > MAX_CHECKS_PER_RULE:
            problems.append(
                f"{rule.id}: {len(rule.checks)} checks exceeds the limit of "
                f"{MAX_CHECKS_PER_RULE}; split the rule so each one stays answerable"
            )
        if rule.tier > 0 and rule.applies_to == ("**/*",):
            problems.append(
                f"{rule.id}: judgment rules must narrow 'applies_to'; '**/*' sends "
                f"every changed file to a model"
            )
        if rule.tier > 0 and not rule.examples.get("allowed"):
            problems.append(
                f"{rule.id}: add at least one '### Allowed' example so the agent can "
                f"tell a violation from a false positive"
            )
        if len(rule.description.split()) > 25:
            problems.append(f"{rule.id}: description should be one short sentence")
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--json", action="store_true", help="machine-readable output")
    parser.add_argument("paths", nargs="*", help="ignored; accepted for pre-commit")
    args = parser.parse_args()

    root = changeset.repo_root()
    problems: list[str] = []
    skipped: list[str] = []
    loaded: list[rulelib.Rule] = []

    try:
        loaded = rulelib.load_rules(root)
    except rulelib.RuleError as exc:
        problems.extend(str(exc).splitlines())

    if loaded:
        problems.extend(extra_checks(loaded))
        problems.extend(check_detector_agrees(loaded))
        unanchored, skipped = check_provenance(root, loaded)
        problems.extend(unanchored)

    waivers = Waivers(root)
    problems.extend(waivers.errors)

    if args.json:
        print(
            json.dumps(
                {
                    "ok": not problems,
                    "rules": len(loaded),
                    "index_hash": rulelib.index_hash(loaded) if loaded else None,
                    "waivers": len(waivers),
                    "problems": problems,
                    "skipped": skipped,
                },
                indent=2,
            )
        )
        return 1 if problems else 0

    if problems:
        print("Rule validation failed:\n", file=sys.stderr)
        for problem in problems:
            print(f"  - {problem}", file=sys.stderr)
        print(
            f"\nSee {root / '.agents/rules/README.md'} for the authoring contract.",
            file=sys.stderr,
        )
        return 1

    for note in skipped:
        print(f"  skipped: {note}", file=sys.stderr)

    tiers = {0: 0, 1: 0, 2: 0}
    sourced = 0
    for rule in loaded:
        tiers[rule.tier] += 1
        sourced += sum(
            1 for group in rule.examples.values() for e in group if e.is_sourced
        )
    print(
        f"{len(loaded)} rules OK "
        f"(tier0={tiers[0]} tier1={tiers[1]} tier2={tiers[2]}, "
        f"{sourced} sourced example(s) verified, "
        f"index={rulelib.index_hash(loaded)}, waivers={len(waivers)})"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
