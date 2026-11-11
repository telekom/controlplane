---
name: code-auditor
description: Use when auditing local or staged changes, reviewing a branch or pull request, or checking a finished feature against this repository's rules.
---

# Code Auditor Skill

Audit the selected change against `.agents/rules`.

Scripts do all the deciding they can. You only answer the questions a script
cannot, and every answer you give is machine-verified before it counts.

## Rules of engagement

- Never edit `.agents/.audit/report.json`. Write answers to `verdicts.json`.
- Never guess a verdict. `UNCLEAR` is a valid answer and is always better than
  an invented one.
- Never add a waiver yourself. Only a human waives a finding.
- Never fix code during the audit. Audit first, report, then fix.

## Auditing a submodule

Resolve the audited Git root as `REPO_ROOT` and the rules-owning root as
`CONFIG_ROOT`, both absolute paths. Set `SCRIPTS_DIR` to
`<CONFIG_ROOT>/.agents/skills/code-auditor/scripts` and `VIEWER_PATH` to
`<SCRIPTS_DIR>/show_work_unit.py`; verify those scripts exist before running.
Run every command from `REPO_ROOT` and pass `--config-root "$CONFIG_ROOT"`.
When the change lives in a submodule whose rules sit in the enclosing
workspace, `CONFIG_ROOT` is that workspace.
State then lives in `<workspace>/.agents/.audit/repos/<submodule>/` instead
of `.agents/.audit/`. See "Shared configuration" in `scripts/README.md`.

## Workflow

### 0. Select the scope

Use the scope already specified by the user's prompt. Otherwise ask with the
question tool before running any audit script:

- **Local changes**: all uncommitted changes against `HEAD`, including
  untracked files; use current working-tree content (`--mode local`).
- **Staged changes**: exactly the index against `HEAD`, excluding unstaged
  edits and untracked files (`--mode staged`).
- **Branch / PR review**: committed changes from the target branch's merge
  base to the reviewed tip, excluding all local edits (`--mode range`).

"Audit", "review my changes", or "feature finished" alone do not select a
scope. A clean working tree does not imply a branch review: ask rather than
silently switching scopes. If no answer is available, stop before generating
or replacing audit state.

For a PR, read its target branch and head commit from PR metadata (for example
`gh pr view <PR> --json baseRefName,headRefOid`), fetch the needed refs, and
verify that the repository contains that head. Pass the target ref with
`--base` and the head commit with `--revision`. Do not assume the target is
`main`. For a branch review without a specified target, use the repository's
configured audit target or default branch; ask if it cannot be resolved.
When the reviewed tip is the current branch's `HEAD`, align its push hook
with the selected target using
`git config branch.<current-branch>.auditTarget <target-ref>`.
For another tip, leave the current branch's configuration unchanged.
See "Snapshot and receipt contract" in `scripts/README.md` for target precedence.

Include branch changes **and** local edits only when explicitly requested
(`--mode worktree --base <target>`). This combined scope is not a fallback.

Before running, announce the selected scope, comparison base/target and tip,
and whether local edits and untracked files are included. Keep that selection
for every regeneration after fixes; change it only on user direction.

### 1. Generate the report

Run the command for the selected scope:

```bash
python3 "$SCRIPTS_DIR/create_audit_report.py" --config-root "$CONFIG_ROOT" --mode local
python3 "$SCRIPTS_DIR/create_audit_report.py" --config-root "$CONFIG_ROOT" --mode staged
python3 "$SCRIPTS_DIR/create_audit_report.py" --config-root "$CONFIG_ROOT" --mode range --base origin/<target> --revision <head-commit>
```

Run only the selected command, not all three. The CLI requires an explicit
mode and prints the resolved comparison. Check that the recorded `mode`,
`base` and `comparison` in the report match the announced scope.
Detectors and evidence validation use the chosen frozen snapshot, including
package context.
Repository-owned rules, waivers, judges and detector inputs come from that
snapshot too. For a staged audit, stage intended configuration changes first;
for a branch audit, commit them first. Shared configuration is frozen and
content-bound to receipts. Ensure `codespell` is on `PATH` before auditing.
Regenerating the report resets its verdict file; reusable judgments come
from the separate cache.

The judge is pinned in `.agents/judges.yaml` and recorded in every receipt.
Never set `AUDIT_MODEL`: the model driving this session is not the model that
answers, and a receipt naming the wrong one is worse than no receipt.

Read `report.json` from the state directory recorded above. If
`stats.items_pending` is `0`, skip to
step 4 — everything was settled deterministically.

If `errors` is non-empty a detector tool is missing. Report that to the user;
do not treat it as a pass.

### 2. Dispatch the work units

`work_units` is already batched and ordered — one unit holds the checks for
one or two candidates of a single rule, so their shared context is sent once.
Start one subagent per work unit, in parallel, each with:

- the judge agent named by the report: `audit-judge-<judge>`. If this harness
  has no such agent, stop and tell the user; never fall back to another model.
- its `unit_id` as `UNIT_ID`, plus the resolved `VIEWER_PATH`, `REPO_ROOT`
  and `CONFIG_ROOT`. Include the exact quoted viewer invocation with these
  paths in each dispatch, including selftest dispatches.

The persona and the pinned model live in those agent definitions, generated
from `.agents/judges.yaml`. Do not paste the persona into the prompt and do
not name a model yourself: a model you type is a model nobody verified.

A subagent must never read `report.json`. It runs:

```bash
cd "$REPO_ROOT"
python3 "$VIEWER_PATH" "$UNIT_ID" --config-root "$CONFIG_ROOT"
```

which prints only its own rule, examples, frozen context and questions.

One subagent handles one work unit and nothing else. Do not merge work units,
do not reorder them, do not reuse a subagent for a second unit — each is a
fresh context on purpose.

### 3. Collect the verdicts

Merge every subagent's answers into `verdicts.json` beside that report:

```json
{
  "verdicts": [
    {
      "item_id": "209d9965ebb46fd4",
      "verdict": "FAIL",
      "evidence": {
        "line": 12,
        "quote": "sort.Strings(keys)",
        "reason": "Plain string slice sort; slices.Sort(keys) replaces it directly."
      }
    }
  ]
}
```

### 4. Settle

```bash
python3 "$SCRIPTS_DIR/process_audit_report.py" --config-root "$CONFIG_ROOT" --fail-on HIGH
```

Exit `0` is clean, `1` means findings, `2` means the audit was **rejected** —
an item was unanswered or a quote could not be found in the file it cited. On
`2`, re-run the affected work units with corrected answers. Do not work around
the rejection.

### 5. Report to the user

Print the findings exactly as the script rendered them: severity, rule, file,
line, quote, reason, waive id. Then offer to fix them.

Anything printed under **advisory** sits outside the change — it is real, but
it is not this change's regression and it does not block. Mention it once; do
not start fixing it unless the user asks.

After a fix, re-run from step 1 — unchanged items come back from cache, so the
second pass is nearly free.
