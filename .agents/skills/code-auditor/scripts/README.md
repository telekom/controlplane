<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Audit engine

Stdlib-only Python, no virtualenv, no network. Everything here is
deterministic; the only non-deterministic step in the whole pipeline is the
agent that fills in `verdicts.json`, and `process_audit_report.py` verifies
its output before accepting it.

| Script | Runs where | Purpose |
| --- | --- | --- |
| `validate_rules.py` | pre-commit, CI | Static validation of `.agents/rules` |
| `create_audit_report.py` | agent session | Changeset → detectors → items → work units |
| `show_work_unit.py` | subagent | Print one work unit: rule, context, questions |
| `process_audit_report.py` | agent session | Verify agent verdicts, write receipts, render findings |
| `audit_gate.py` | pre-commit, pre-push, CI | Prove an audit covers the staged blobs |
| `sync_judges.py` | pre-commit | Keep the harness judge agents equal to the pins |
| `selftest_rules.py` | pre-commit (`--static`), CI | Grade the rules themselves: accuracy, flake rate, cost |

Work units are packed **candidate-major**: every check of one candidate rides
in the same prompt, so its frozen context is sent once instead of once per
check. Severity sets two caps, on candidates and on resulting questions.

`auditlib/` holds the shared pieces: `yamlish` (frontmatter subset),
`rules` (parse and index), `provenance` (example sources), `changeset`
(git scope), `detect` (candidates), `report` (schema, item ids, batching) and
`store` (cache, receipts, waivers).

Two things are re-anchored against real files, and both reject rather than
warn: an agent's evidence quote (`process_audit_report.py`) and a rule's
example provenance (`validate_rules.py`).

## Rule selftest

Sourced examples are fixtures: a `### Violation` must be caught, an
`### Allowed` must not. Three phases:

```bash
selftest_rules.py --static                    # no model, ~0.2s, runs on every commit
selftest_rules.py --plan --samples 3          # fixture work units, one per sample
selftest_rules.py --score --max-flake 0.05    # accuracy and flake rate per rule
```

`--static` needs no model at all: it reports candidates and **model questions
per 100 files in scope**, which is the number that actually predicts cost, and
fails a rule whose detector is too broad.

When a fixture is judged, its own example is **removed from the prompt**.
Without that hold-out the agent is shown the answer and the test measures
nothing. It also exposes a specific failure worth catching: an exception that
only survives because its example is in the prompt, rather than because the
rule's prose justifies it.

Accuracy and flake rate are separate axes. A rule can be perfectly repeatable
and perfectly wrong.

## State

`.agents/.audit/` is gitignored, local, and safe to delete at any time —
deleting it only costs re-judging.

| File | Contents |
| --- | --- |
| `report.json` | the current audit |
| `verdicts.json` | the agent's answers, merged by the processor |
| `receipts.json` | per (file blob, rule) proof used by the commit gate |
| `verdicts.cache.json` | content-addressed verdict reuse |
| `rules.index.json` | compiled rule index, keyed on the rules directory |
| `state.json` | model and rule index hash of the last audit |

`.agents/audit-waivers.yaml` is the one piece of state that *is* committed.

## Exit codes

| Code | `create` / `process` / `gate` |
| --- | --- |
| 0 | clean |
| 1 | findings at or above the threshold, or the gate blocked |
| 2 | engine error: invalid ruleset, or the agent's output was rejected |

## Manual use

```bash
# validate the ruleset
python3 validate_rules.py

# what would be audited, without judging anything
python3 create_audit_report.py --mode worktree

# check the gate without committing
python3 audit_gate.py --mode staged
```

## Judges

`.agents/judges.yaml` pins the judges by tier, as `provider/model#effort`. The
resolved pin is part of every cache and receipt key, so a receipt names the
model that actually answered rather than the one that asked.

```bash
sync_judges.py            # regenerate the harness agent definitions
sync_judges.py --check    # pre-commit: refuse a commit where they drifted
```

The pin and the harness are two places holding one fact. A subagent whose
definition carries no model silently inherits the session's model, so the
check is what stops a receipt from naming a judge that never ran.

A harness only gets a judge it can run under the pinned name:

| Harness | Location | Generated for |
|---|---|---|
| OpenCode | `.opencode/agents/audit-judge-<tier>.md` | every pin, `model: provider/model#effort` |
| Claude Code | `.claude/agents/audit-judge-<tier>.md` | `anthropic/*` pins |
| VS Code GitHub Copilot | `.github/agents/audit-judge-<tier>.agent.md` | `github-copilot/*` pins, `model: <model>`, `reasoning-effort: <effort>` |

VS Code applies `reasoning-effort` only in the Copilot harness. The Local
harness uses the agent's `model` for a subagent, unless the main agent passes
its own model to `runSubagent`; `reasoning-effort` is documented for Copilot only, so a judge run
there may not match the effort in its pin. A harness without a definition for
the pinned tier has no judge: switch harness or tier rather than letting the
session model answer.

A generated definition no pin produces any more, such as a Claude judge after
its pin moved to another provider, is removed on write and reported by
`--check`. Files without the generated marker are never touched. Tests:
`python3 -m unittest discover -s .agents/skills/code-auditor/scripts/tests`.

`AUDIT_JUDGE=<tier>` selects another declared tier for one run. There is no
environment variable for a raw model id: a pin a shell can rewrite is not a
pin. `AUDIT_GATE=off` bypasses the gate.
