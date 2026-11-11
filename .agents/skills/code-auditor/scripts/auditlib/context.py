# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Where an audit reads its configuration and where it keeps its state.

The audited repository is always the git root of the working directory: its
changeset, its files and its tools. The configuration - ``.agents/rules``,
``.agents/judges.yaml`` and ``.agents/auditignore`` - normally lives in
that same repository. A config root lets a submodule be audited with the rules
of the workspace that contains it.

The config root must be the audited repository or one of its ancestors, so a
shared configuration can only come from an enclosing workspace. With a shared
configuration, audit state lives in the config root under
``.agents/.audit/repos/<repo path relative to the config root>`` so the
audited repository gains no untracked files and two repositories never share a
cache, receipts or report.
"""

from __future__ import annotations

import argparse
import os
from dataclasses import dataclass
from pathlib import Path

from . import changeset

CONFIG_ROOT_ENV = "AUDIT_CONFIG_ROOT"
AUDIT_DIRNAME = ".audit"
SHARED_DIRNAME = "repos"


class ConfigError(ValueError):
    """Raised when the requested config root cannot serve this repository."""


@dataclass(frozen=True)
class AuditContext:
    repo_root: Path
    config_root: Path

    @property
    def shared(self) -> bool:
        return self.config_root != self.repo_root

    @property
    def audit_dir(self) -> Path:
        base = self.config_root / ".agents" / AUDIT_DIRNAME
        if not self.shared:
            return base
        return base / SHARED_DIRNAME / self.repo_root.relative_to(self.config_root)

    def describe(self) -> dict:
        return {"repo_root": str(self.repo_root), "config_root": str(self.config_root)}


def add_argument(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "--config-root",
        type=Path,
        default=None,
        help=(
            "directory whose .agents holds the rules, judges and waivers; must "
            f"be the audited git root or an ancestor of it (env: {CONFIG_ROOT_ENV})"
        ),
    )


def resolve(config_root: Path | None = None, start: Path | None = None) -> AuditContext:
    """Resolve the audit context, preferring the explicit option over the env."""
    repo = changeset.repo_root(start).resolve()
    requested = config_root or _from_env()
    if requested is None:
        return AuditContext(repo_root=repo, config_root=repo)

    config = requested.expanduser().resolve()
    if not config.is_dir():
        raise ConfigError(f"config root {config} is not a directory")
    if config != repo and config not in repo.parents:
        raise ConfigError(
            f"config root {config} is neither the audited repository {repo} "
            "nor one of its ancestors"
        )
    agents = config / ".agents"
    for required in (agents / "rules", agents / "judges.yaml"):
        if not required.exists():
            raise ConfigError(
                f"config root {config} has no {required.relative_to(config)}"
            )
    return AuditContext(repo_root=repo, config_root=config)


def _from_env() -> Path | None:
    value = os.environ.get(CONFIG_ROOT_ENV, "").strip()
    return Path(value) if value else None
