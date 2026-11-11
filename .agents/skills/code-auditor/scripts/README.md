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
| `validate_rules.py` | pre-commit | Static validation of `.agents/rules` |
| `create_audit_report.py` | agent session | Changeset → detectors → items → work units |
| `show_work_unit.py` | subagent | Print one work unit: rule, context, questions |
| `process_audit_report.py` | agent session | Verify agent verdicts, write receipts, render findings |
| `audit_gate.py` | pre-commit, Git pre-push | Prove an audit covers staged or committed blobs |
| `sync_judges.py` | pre-commit | Keep the harness judge agents equal to the pins |
| `selftest_rules.py` | pre-commit (`--static`), manual model-backed run | Grade accuracy, flake rate, cost |

Work units are packed **candidate-major**: every check of one candidate rides
in the same prompt, so its frozen context is sent once instead of once per
check. Severity sets two caps, on candidates and on resulting questions.

`auditlib/` holds the shared pieces: `yamlish` (frontmatter subset),
`rules` (parse and index), `provenance` (example sources), `changeset`
(git scope), `context` (config root and state location), `detect` (candidates), `report` (schema, item ids, batching) and
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

Fixture IDs are opaque and expected outcomes stay in scoring-only metadata.
Allowed fixtures require explicit `PASS` answers to every check; `UNCLEAR`
is not a correct answer. The viewer automatically resolves `selftest-*` unit
IDs from `.agents/.audit/selftest/plan.json`. An explicit `--report` overrides
that location. Creating a new plan resets its verdict file.

## Snapshot and receipt contract

The session audit compares against the default branch's merge base, including
already-pushed feature commits. `--mode staged` exports the complete index;
the push gate's `--mode range` exports committed `HEAD`, ignoring local edits.
`--base` and `--revision` select explicit committed endpoints for the gate.
Detector tools run inside these isolated trees with package context intact.
Evidence is verified against the recorded blobs in Git's object store.

Judgment reuse includes the file path, numbered context, change markers and
detector detail. Receipts additionally bind to the file's changed-line scope
and complete dependency tree hash. Only applicable file/rule pairs receive
receipts. Schema 3 invalidates older reports and receipts; regenerate them.
Both deterministic checks and model judgments honor `auditignore`.

Deletion boundaries count as touched lines. Judgment detectors discover
triggers within `context_lines` of edits, including deletions, while
file-scoped judgments receive the complete file. Evidence in that affected
window can block; distant pre-existing findings remain advisory. Spec-field
tools report a declaration/comment span so comment-only changes are covered.

Missing tools, crashes and invalid receipts are engine errors, never passes.
`output_lines` tools must exit 0; codespell's documented findings status 65
is also accepted. A report containing errors cannot write receipts.
Regenerating a report resets `verdicts.json`; reusable judgments live in the
separate context-addressed cache.

Run `pre-commit install` to install pre-commit, commit-msg and pre-push hooks.
Model-backed cold CI auditing is planned and is not currently enforced.

## State

`.agents/.audit/` is gitignored, local, and safe to delete at any time —
deleting it only costs re-judging.

| File | Contents |
| --- | --- |
| `report.json` | the current audit |
| `verdicts.json` | the agent's answers, merged by the processor |
| `receipts.json` | path, blob, rule, change-scope and dependency-tree proof |
| `verdicts.cache.json` | content-addressed verdict reuse |
| `rules.index.json` | compiled rule index, keyed on the rules directory |
| `state.json` | selected judge identity, model pin and rule index hash |

`.agents/auditignore` is the one piece of state that *is* committed. Despite
its extensionless name, it is YAML: each finding's item id maps to a reason,
not a file pattern. Quote reasons that contain YAML punctuation.

## Shared configuration

A submodule can be audited with the rules of the workspace that contains it.
Run from the submodule and pass the workspace as config root, either with
`--config-root DIR` or `AUDIT_CONFIG_ROOT=DIR`; the option wins over the
variable. `create_audit_report.py`, `show_work_unit.py`,
`process_audit_report.py` and `audit_gate.py` all accept it, and all four
must get the same value.

```bash
cd controlplane
python3 ../.agents/skills/code-auditor/scripts/create_audit_report.py --config-root ..
python3 ../.agents/skills/code-auditor/scripts/show_work_unit.py '<unit_id>' --config-root ..
python3 ../.agents/skills/code-auditor/scripts/process_audit_report.py --config-root .. --fail-on HIGH
```

| Comes from | What |
| --- | --- |
| audited git root | changeset, file contents, detector working directory, paths in `applies_to` |
| config root | `.agents/rules`, `.agents/judges.yaml`, `.agents/auditignore`, detector scripts and data under `.agents/` |
| `<config root>/.agents/.audit/repos/<repo path>/` | every file listed under State |

Detector command arguments beginning with `.agents/` (including option values
such as `--ignore-words=.agents/codespell-ignore.txt`) resolve against the config
root. Other relative arguments and input file paths resolve against the audited
repository.

The config root must be the audited git root or one of its ancestors and must
hold `.agents/rules` and `.agents/judges.yaml`; anything else exits 2 before a
file is written. The report records both roots, and `process_audit_report.py`
rejects a report created for another pair. Without a config root the audited
git root is its own config root and state stays in `.agents/.audit/`.

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

`.agents/judges.yaml` pins the judges by complexity, as `provider/model#effort`. The
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
| OpenCode | `.opencode/agents/audit-judge-<complexity>.md` | every pin, `model: provider/model`, `variant: <effort>` |
| Claude Code | `.claude/agents/audit-judge-<complexity>.md` | `anthropic/*` pins |
| VS Code GitHub Copilot | `.github/agents/audit-judge-<complexity>.agent.md` | `github-copilot/*` pins, `model: <model>`, `reasoning-effort: <effort>` |

The `#effort` suffix belongs to the pin only: it keeps receipts and cache keys
distinct per effort. Harnesses get model and effort as separate fields. The
OpenCode Markdown agent loader (verified on 1.18.30) reads `variant`
separately and does not parse `#effort` inside `model`. Restart OpenCode after
regenerating, since a running session keeps the agent definitions it loaded.

VS Code applies `reasoning-effort` only in the Copilot harness. The Local
harness uses the agent's `model` for a subagent, unless the main agent passes
its own model to `runSubagent`; `reasoning-effort` is documented for Copilot only, so a judge run
there may not match the effort in its pin. A harness without a definition for
the pinned complexity has no judge: switch harness or complexity rather than letting the
session model answer.

A generated definition no pin produces any more, such as a Claude judge after
its pin moved to another provider, is removed on write and reported by
`--check`. Files without the generated marker are never touched. Tests:
`python3 -m unittest discover -s .agents/skills/code-auditor/scripts/tests`.

`AUDIT_JUDGE=<complexity>` selects another declared complexity for one run. There is no
environment variable for a raw model id: a pin a shell can rewrite is not a
pin. `AUDIT_GATE=off` bypasses the gate.
