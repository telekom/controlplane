# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""The judges an audit may dispatch to, and the pin recorded in every key.

Nothing about a run may depend on which model happens to be driving the
session. The judge is declared in ``.agents/judges.yaml``, resolved here, and
written into every cache and receipt key, so a receipt states who actually
answered rather than who asked.

A pin is a model *and* an effort level. Dropping the effort would make two
measurably different functions share one key space.
"""

from __future__ import annotations

import os
import re
from dataclasses import dataclass
from pathlib import Path

from .yamlish import YamlishError, parse as parse_yamlish

JUDGES_FILENAME = "judges.yaml"
DEFAULT_KEY = "default"

#: provider/model with an optional '#effort'. Deliberately strict: a typo that
#: resolves to something plausible is worse than one that fails here.
PIN_PATTERN = re.compile(r"^([a-z0-9][\w.-]*/[\w.:-]+)(?:#([\w-]+))?$")


class JudgeError(ValueError):
    """Raised when the judge pins cannot be resolved.

    Never degrades to a default. An audit that cannot name its judge cannot
    write an honest receipt, and a receipt nobody can trust is worse than no
    receipt at all.
    """


@dataclass(frozen=True)
class Judge:
    tier: str
    model: str
    effort: str

    @property
    def pin(self) -> str:
        return f"{self.model}#{self.effort}" if self.effort else self.model


@dataclass(frozen=True)
class Judges:
    default: str
    by_tier: dict[str, Judge]

    @property
    def tiers(self) -> tuple[str, ...]:
        return tuple(sorted(self.by_tier))

    def get(self, tier: str) -> Judge:
        judge = self.by_tier.get(tier)
        if judge is None:
            known = ", ".join(self.tiers) or "none"
            raise JudgeError(f"unknown judge tier '{tier}'; known tiers: {known}")
        return judge

    def resolve(self, tier: str | None = None) -> Judge:
        """The judge to use, honouring the AUDIT_JUDGE tier override."""
        return self.get(tier or os.environ.get("AUDIT_JUDGE", "").strip() or self.default)


def judges_path(repo_root: Path) -> Path:
    return repo_root / ".agents" / JUDGES_FILENAME


def load(repo_root: Path) -> Judges:
    path = judges_path(repo_root)
    if not path.is_file():
        raise JudgeError(f"{path} is missing; an audit has no judge to run on")

    try:
        raw = parse_yamlish(path.read_text(encoding="utf-8"))
    except YamlishError as exc:
        raise JudgeError(f"{JUDGES_FILENAME}: {exc}") from exc

    default = str(raw.pop(DEFAULT_KEY, "")).strip()
    if not default:
        raise JudgeError(f"{JUDGES_FILENAME}: missing 'default'")

    by_tier: dict[str, Judge] = {}
    for tier, value in raw.items():
        match = PIN_PATTERN.match(str(value).strip())
        if match is None:
            raise JudgeError(
                f"{JUDGES_FILENAME}: judge '{tier}' is not a pin of the form "
                f"'provider/model#effort': {value!r}"
            )
        by_tier[tier] = Judge(tier=tier, model=match.group(1), effort=match.group(2) or "")

    if not by_tier:
        raise JudgeError(f"{JUDGES_FILENAME}: no judges declared")
    if default not in by_tier:
        raise JudgeError(
            f"{JUDGES_FILENAME}: default '{default}' is not one of "
            f"{', '.join(sorted(by_tier))}"
        )

    return Judges(default=default, by_tier=by_tier)
