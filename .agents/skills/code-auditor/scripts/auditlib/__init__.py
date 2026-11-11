# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Deterministic rule-engine for the code-auditor skill.

The library is intentionally dependency-free (stdlib only) so that it can run
inside a git hook, inside CI and inside an agent session without a virtualenv.
"""

SCHEMA_VERSION = 3
SEVERITIES = ("LOW", "HIGH", "CRITICAL")
VERDICTS = ("PASS", "FAIL", "UNCLEAR")

__all__ = ["SCHEMA_VERSION", "SEVERITIES", "VERDICTS"]
