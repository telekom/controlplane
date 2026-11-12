---
description: Canonical ordered ruleset enforced by the code-auditor skill.
---

# Agent Rules

Every file in this directory is one rule. `validate_rules.py` enforces the
contract below on every commit, so a rule that does not parse cannot be merged.

The guiding principle: **push as much of the rule below the LLM line as you
can, and declare what is left.** A rule a script can decide is free, instant
and perfectly repeatable. A rule a model has to judge costs tokens and varies
between runs. Decompose until the model's share is small.

## Complexities

| Complexity | Who decides | Required sections |
| --- | --- | --- |
| 0 | the detector alone, no model involved | `## Detect` |
| 1 | a detector finds candidates, a model judges each one | `## Detect`, `## Checks`, `## Examples` |
| 2 | no detector is possible, a model judges every changed file | `## Checks`, `## Examples` |

Complexity 2 is a last resort. Before reaching for it, ask whether a regex over the
changed lines could at least narrow the files.

Complexities are not permanent. `01-prefer-slices-sort` started at complexity 1 because the
legacy `sort` package looked like it had genuine exceptions; the selftest
showed that every call in this repository converts, so it dropped to complexity 0 and
now costs nothing. Run the selftest before assuming a rule needs judgement.

## Frontmatter

| Field | Required | Notes |
| --- | --- | --- |
| `id` | yes | `NN-kebab-case`, must equal the filename without `.md` |
| `description` | yes | one short sentence, max 25 words |
| `severity` | yes | `LOW`, `HIGH` or `CRITICAL` |
| `complexity` | yes | `0`, `1` or `2` |
| `scope` | no | candidate granularity: `line`, `file` (default) or `changeset` |
| `report_scope` | no | which lines may be reported: `changed` (default) or `file` |
| `applies_to` | yes | list of globs; `**/*` is rejected for complexity 1 and 2 |
| `exclude` | no | list of globs, applied after `applies_to` |
| `context_lines` | no | lines of frozen context around a candidate, default 20 |
| `report_radius` | no | judgment evidence distance from touched lines, 0..200; defaults to `context_lines` |

Severity drives everything downstream: batch size, whether the commit gate
blocks, and whether a `FAIL` is re-sampled. Pick it deliberately.

### `scope` versus `report_scope`

They are orthogonal and both matter.

`scope` is how much of a file one candidate covers. `line` makes every match
its own candidate (one typo, one finding). `file` collapses a file's matches
into a single candidate judged as a whole.

`report_scope` is which lines are allowed to produce a finding:

- `changed` (default) — only lines the change touched. A rule still *reads*
  surrounding code for context, it just cannot bill you for code you did not
  write. Inheriting a file's backlog because you edited one line in it is how
  linters get switched off.
- `file` — the whole file. Reserve this for rules about document-level
  coherence, where a finding on an untouched line is the point.

  Be slow to reach for it. `04-over-technical-docs` started here, and the
  selftest showed why it is dangerous: the rule told the judge to weigh the
  whole document while the prompt marked the changed lines, so the same
  fixture came back `FAIL PASS PASS`. If a check can be phrased as "do the
  marked lines do X, given the rest of the document", it belongs in
  `changed`.

For judgments with `report_scope: changed`, `report_radius` controls how far
evidence may sit from a touched line. Set it to `0` for marked-lines-only
checks such as `04-over-technical-docs`. This is independent of the context
shown to the judge: `scope: file` still supplies the complete file.

Findings outside the change are not discarded: they are reported as
**advisory** and excluded from the gate.

Deletion boundaries are marked as touched. Judgment detectors also discover
existing triggers within `context_lines` of a touched line; this catches
removed guards or labels without scanning unrelated distant code. Evidence
in that affected window may block. `scope: file` supplies the complete file
to the judge rather than discarding distant matches after the first window.
Tool detectors can append `[audit-span=START-END]` to a finding to associate
a declaration with its comments; edits anywhere in that span count.

## Sections

### `## Detect`

Exactly one fenced block. Either a regex over candidate lines:

````md
```regex
\bsort\.(Sort|Slice)\(
```
````

or a tool whose output locates the violations:

````md
```tool
command: python3 .agents/tools/spelling.py --ignore-words=.agents/codespell-ignore.txt --
violation_when: output_lines
output_format: jsonl
```
````

`violation_when` is `output_lines` or `nonzero_exit`.
For `output_lines`, use `output_format: jsonl`: each output line is a JSON
object with `file` (exact input path), `line` (positive integer) and `detail`
(string), optionally `span: [start, end]` for declaration/comment coverage.
JSON escaping preserves filenames containing colons, whitespace or newlines.
Built-in detectors use this format. The spelling adapter feeds each file's
bytes to codespell through stdin and attaches its original path to each result.
Legacy `output_format: text` (the default) accepts `file:line:message` for
single-line filenames. Malformed records, unknown paths and invalid line/span
bounds produce engine errors instead of being silently dropped.
A tool that is not installed produces an engine error, never a silent pass.

Keep detectors tight. A pattern that matches a hundred lines per change is a
cost and a noise problem, and the rule selftest will flag it.

### `## Checks`

A flat bullet list. Each item is one closed question the model answers
independently:

```md
- [observed-generation] Is `ObservedGeneration` stamped on the condition?
```

The slug must be unique within the rule and the question must end in `?`.
Maximum eight checks — more than that means the rule is really two rules.

A check may declare the signal that makes it worth asking:

```md
- [fixed-requeue] when=`(RequeueAfter|Requeue:)` Does the change requeue on a fixed interval instead of a jittered one?
```

If the pattern is absent from the candidate's context window, the engine
answers `PASS` itself and records why. Paying a model to confirm that a string
is missing is pure cost: on `02-no-reconcile-loops` these triggers cut the
questions per 100 in-scope files from 225 to 55.

Two constraints, both enforced:

- `when` is optional. A check without one always runs, so narrowing is a
  deliberate act per check, never a silent default.
- The rule's `## Detect` pattern must stay at least as broad as the union of
  the `when` patterns, or a check could never be reached. The selftest fails
  the rule if an example shows otherwise.

A trigger no example exercises is reported by `selftest_rules.py --static` as
an untested check. That is a gap in the fixtures, not an error.

### `## Examples`

`### Violation` and `### Allowed` subsections holding fenced blocks. These are
injected verbatim into the judgment prompt and are the single strongest lever
for making a verdict repeatable.

**Every example must declare where it came from.** "Write them from real code"
is advice; provenance makes it a check.

````md
```go source=admin/internal/handler/zone/links.go:52@b53a2d9c
sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
```
````

`validate_rules.py` runs `git show <rev>:<path>` and re-anchors every
significant line of the block near the cited line, requiring exact equality
after whitespace normalization. Elision-only lines (`...`) are skipped;
truncating a source statement is not elision. A wrong line, an invented
snippet or a drifting quote all fail the commit.

The revision is pinned deliberately. Violation examples describe code the
project wants gone, so a reference to a moving branch would rot the moment
someone fixes it. Use the current `HEAD` short sha when you write the rule and
it stays valid forever.

Some violations genuinely have no instance to cite — a reconcile loop in
`main` would be an incident, not an example. Those are declared, not omitted:

````md
```go synthetic="a committed reconcile loop would be a production incident; this is the shape being prevented"
```
````

Rules for the two:

- at least one example per rule must be `source=`, so no rule ships entirely
  uncalibrated against this codebase
- `### Allowed` examples should always be sourced — code that is fine today
  exists by definition
- a `synthetic` block without a reason is rejected

Two further checks run on examples:

- a regex detector must match its own `### Violation` examples, otherwise the
  rule can never fire
- a revision missing from a shallow clone is reported as skipped, never as a
  failure, so CI checkout depth cannot break an honest rule

## Template

````md
---
id: 06-my-new-rule
description: One sentence on what this rule protects.
severity: HIGH
complexity: 1
scope: line
applies_to:
  - "**/*.go"
exclude:
  - "**/*_test.go"
---

# My New Rule

Why the rule exists and what a reader needs to know to apply it.

## Detect

```regex
some-narrow-pattern
```

## Checks

- [slug] Is the closed question a reviewer would ask here?

## Examples

### Violation

```go source=path/to/real/file.go:42@abc1234
// real code that breaks the rule
```

### Allowed

```go source=path/to/other/file.go:17@abc1234
// real code that looks similar but is fine
```
````

Get the revision with `git rev-parse --short HEAD`.

## Audit ignores

An accepted finding is recorded in `.agents/auditignore` as a YAML mapping,
keyed by the item id the audit prints. Values are human-written reasons, not
file patterns:

```yaml
a1b2c3d4e5f6a7b8: Reverse sort has no slices equivalent. (ron, 2026-11-09)
```

Audit ignores expire by construction — the id contains the snippet, so changing
the code retires the audit ignore automatically.
Each reason must be a nonempty string; empty values, lists, booleans and
numbers are rejected. Quote numeric-looking reasons if they are intended as text.
