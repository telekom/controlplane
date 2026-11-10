---
name: code-auditor
description: Use me when you have finished implemented a feature to audit the current working-tree code diffs against rules for this repository.
---

# Code Auditor Skill

Audit the current change against `.agents/rules` before it is committed.

Scripts do all the deciding they can. You only answer the questions a script
cannot, and every answer you give is machine-verified before it counts.

## Rules of engagement

- Never edit `.agents/.audit/report.json`. Write answers to `verdicts.json`.
- Never guess a verdict. `UNCLEAR` is a valid answer and is always better than
  an invented one.
- Never add a waiver yourself. Only a human waives a finding.
- Never fix code during the audit. Audit first, report, then fix.

## Workflow

### 1. Generate the report

```bash
python3 .agents/skills/code-auditor/scripts/create_audit_report.py
```

The judge is pinned in `.agents/judges.yaml` and recorded in every receipt.
Never set `AUDIT_MODEL`: the model driving this session is not the model that
answers, and a receipt naming the wrong one is worse than no receipt.

Read `.agents/.audit/report.json`. If `stats.items_pending` is `0`, skip to
step 4 — everything was settled deterministically.

If `errors` is non-empty a detector tool is missing. Report that to the user;
do not treat it as a pass.

### 2. Dispatch the work units

`work_units` is already batched and ordered — one unit holds the checks for
one or two candidates of a single rule, so their shared context is sent once.
Start one subagent per work unit, in parallel, each with:

- the judge agent named by the report: `audit-judge-<judge>`. If this harness
  has no such agent, stop and tell the user; never fall back to another model.
- its `unit_id`

The persona and the pinned model live in those agent definitions, generated
from `.agents/judges.yaml`. Do not paste the persona into the prompt and do
not name a model yourself: a model you type is a model nobody verified.

A subagent must never read `report.json`. It runs:

```bash
python3 .agents/skills/code-auditor/scripts/show_work_unit.py '<unit_id>'
```

which prints only its own rule, examples, frozen context and questions.

One subagent handles one work unit and nothing else. Do not merge work units,
do not reorder them, do not reuse a subagent for a second unit — each is a
fresh context on purpose.

### 3. Collect the verdicts

Merge every subagent's answers into `.agents/.audit/verdicts.json`:

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
python3 .agents/skills/code-auditor/scripts/process_audit_report.py --fail-on HIGH
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
