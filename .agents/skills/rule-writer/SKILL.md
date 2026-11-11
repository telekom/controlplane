---
name: rule-writer
description: Use me when then user requested to add a new rule to the repositories rulesets.
---

# Rule Writer

You must never use me yourself. This action must always be triggered by a user.

Your job is to write a new rule to `.agents/rules` — if it does not already
exist or is covered by an existing rule. Follow the contract in
[`.agents/rules/README.md`](../../rules/README.md).

## Before writing anything

1. Read every existing rule id and description. If the request is already
   covered, say so and stop.
2. Ask what the rule should protect against, and ask for one real example from
   this repository that violates it. A rule written without a real example is
   a rule nobody can calibrate.

## Pick the lowest complexity that works

This is the only decision that really matters. Work down the list and stop at
the first one that applies:

1. **Can a tool decide it?** (`codespell`, `golangci-lint`, a grep) → complexity 0,
   no model, no cost, perfectly repeatable.
2. **Can a pattern find the candidates, even loosely?** → complexity 1. The pattern
   does not have to be precise; it only has to be a superset. The model
   handles precision.
3. **Neither?** → complexity 2, and narrow `applies_to` as hard as you can.

Before accepting complexity 2, try once more to split the rule. Most "subjective"
rules are one genuinely subjective criterion bolted onto three mechanical
ones — and the mechanical ones belong in their own complexity 0 rule. That is why
`04-over-technical-docs` and `05-docs-no-code-blocks` are two files.

## Writing the checks

Each check is one closed question, answerable yes or no from a window of
source, independent of the other checks. If a check needs the answer to
another check, they are one check. If a rule needs more than eight, it is more
than one rule.

Give each check the signal that makes it worth asking:

```md
- [fixed-requeue] when=`(RequeueAfter|Requeue:)` Does the change requeue on a fixed interval instead of a jittered one?
```

The engine answers a check whose pattern is absent without calling a model.
Keep the rule's `## Detect` at least as broad as the union of these patterns,
or the check can never be reached — the selftest will tell you if it is not.

## Writing the examples

Examples are not illustrations, they are evidence. Every block must cite where
it came from, and `validate_rules.py` re-reads the cited file at the pinned
revision to confirm it:

````md
```go source=admin/internal/handler/zone/links.go:52@b53a2d9c
sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
```
````

So you have to go and find real code:

```bash
git rev-parse --short HEAD          # the rev to pin
grep -rn "<pattern>" --include="*.go" | head
```

Copy the line verbatim. Do not retype it, do not tidy it, do not shorten
identifiers — the check compares against the file and will reject a
paraphrase.

If a violation genuinely has no instance in the repository — a reconcile loop
in `main` would be an incident, not an example — mark it and say why:

````md
```go synthetic="a committed reconcile loop would be a production incident; this is the shape being prevented"
```
````

At least one example in the rule must still be sourced. If you cannot find a
single real line anywhere that relates to the rule, that is a signal: either
the rule is about something this codebase does not do, or you have not looked
hard enough. Ask the user before falling back to all-synthetic.

The `### Allowed` example matters most, and it should always be sourced: it
should look similar enough to the violation that a careless reader would
confuse them. That is what stops false positives.

## Before you finish

```bash
python3 .agents/skills/code-auditor/scripts/validate_rules.py
python3 .agents/skills/code-auditor/scripts/selftest_rules.py --static
```

`--static` prints the rule's cost without calling a model: `quest/100` is the
model questions per 100 in-scope files, `exposure` the total if every in-scope
file were touched. It fails a rule that is too broad.

For a new judgement rule, also grade it against its own examples:

```bash
python3 .agents/skills/code-auditor/scripts/selftest_rules.py --plan --samples 3
# dispatch the printed work units, collect verdicts, then:
python3 .agents/skills/code-auditor/scripts/selftest_rules.py --score
```

Dispatch each `selftest-*` unit through the same judge as an ordinary audit.
The viewer automatically loads its plan instead of the production report;
write the answers to `.agents/.audit/selftest/verdicts.json`.

Fixtures are judged with their own example removed from the prompt, so this
measures whether the rule generalises rather than whether the agent can
pattern-match. Accuracy below 100% means the rule is wrong, not the model —
look at the rule's prose and its allowed examples before blaming the agent.

Then tell the user the new rule's id, complexity, severity, cost and selftest
result, and let them decide whether to keep it.
