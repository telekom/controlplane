---
name: review-architecture
description: Use when reviewing a change, plan or design against the architecture decisions (ADRs), deciding whether a change needs a new ADR, recording a decision, or backfilling ADRs for existing architecture.
---

# Review Architecture

Architecture decision records (ADRs) live in `docs/docs/decisions/`. Each one
records a single decision, the options that lost, and the files it governs.
[`scripts/adr.py`](./scripts/adr.py) does everything a script can decide:
`validate`, `list`, `new`, `match` (which ADRs govern a change or paths) and
`signals` (hints that a change carries a decision). You judge the rest.

Run it from the repository root:

```bash
python3 .agents/skills/review-architecture/scripts/adr.py --help
```

## Trust

A wrong ADR misleads every agent that reads it after you, so trust is the
whole game.

- Only `accepted` ADRs are constraints. A `proposed` ADR is unverified: name
  it, never enforce it. A `rejected` ADR records an idea that was turned
  down: flag a change that brings it back.
- A human sets `accepted` and `rejected`. You write `proposed`, and you
  never rewrite the decision of an accepted ADR: a new ADR supersedes it.
- Evidence or silence. Every claim cites an ADR, a `path:line` or a source.
  When the reason behind a choice is not written down anywhere, say it is
  unknown and ask. A plausible invented rationale is the worst output this
  skill can produce.
- Advisory. Findings inform the author and the reviewer; they never block,
  and you never change code to match an ADR without being asked.

## Pick the mode

| The user wants to… | Mode |
| --- | --- |
| check a change, branch, PR or plan against the architecture | [Review](#review) |
| know whether a change needs an ADR | [Gate](#gate) |
| write down a decision being made now | [Record](#record) |
| write ADRs for decisions made in the past | [Backfill](./BACKFILL.md) |

A review always ends with the gate.

## Review

1. **Scope.** A change is a mode (`worktree` for the current branch,
   `staged`, `local`, or `range` with `--base`/`--revision` for a PR). A plan
   is the list of paths or directories it will touch.
2. **Brief.** Dispatch the `architecture-reviewer` subagent with the scope
   and wait for its brief. Without subagents, follow
   [`reviewer/README.md`](./reviewer/README.md) yourself.
3. **Verdicts.** For every accepted ADR in the brief, read the ADR in full,
   then the changed lines it governs, and give exactly one verdict:
   - **follows**: the change is consistent with the decision.
   - **reopens**: the change departs from the decision and argues for it.
     Phrase it as _"Contradicts ADR-NNNN, but worth reopening because…"_ and
     suggest a superseding ADR ([Record](#record)).
   - **contradicts**: the change departs from the decision without saying
     why. Quote the ADR sentence and the `path:line`.
   - **not affected**: the glob matched, but the decision does not bear on
     what changed. Say why in one line.
4. **Rejected ideas.** For every rejected ADR in the brief, check whether the
   change brings the rejected option back.
5. **Gate** the same scope.

Done when every accepted and rejected ADR in the brief has a verdict with
evidence.

## Gate

1. Run `adr.py signals --mode <mode>`. New modules, new dependencies between
   domains, new third-party libraries and new API kinds are hints, not
   verdicts.
2. Take every signal, every gap in the brief, and every **reopens** verdict,
   and apply the three criteria. An ADR is needed only when all three hold:
   - **Hard to reverse**: changing course later costs real effort.
   - **Surprising without context**: a future reader would ask "why on earth
     like this?"
   - **A real trade-off**: there were genuine alternatives.
3. For each item, answer "needs an ADR" or name the criterion that fails.
   Offer to [record](#record) the ones that need one.

When a reviewer turns a proposal down for a reason a future agent would need,
offer to record that too, so the idea is not suggested again.

## Record

1. **Scan first.** Run `adr.py list` and `adr.py match` on the affected paths,
   and read the architecture page for the domain. Context before questions.
2. **Interview**, one question at a time, until you can state the problem,
   the chosen option, and at least one real alternative with the reason it
   lost. Never fill a gap with your own guess.
3. **Confirm.** Write a three-line intent summary and wait for the user to
   confirm it before creating any file.
4. **Draft.** Run `adr.py new "Title" --domain <dir>` and write the record as
   [ADR-FORMAT.md](./ADR-FORMAT.md) describes.
5. **Validate.** Run `adr.py validate` until it passes.
6. **Hand over.** Leave the status `proposed` and tell the user the record
   waits for their acceptance.

To replace a decision, the new ADR lists the old one in `supersedes`. The old
ADR stays in force until a human accepts the new one and marks the old one
`superseded`; the validator checks that both sides agree.

## Report

```md
## Architecture review: <scope>

Applies: <n> accepted, <n> rejected, <n> proposed (unverified, not checked)

| ADR | Verdict | Evidence |
| --- | --- | --- |
| ADR-0004 Environments isolate tenants | follows | gateway/internal/handler/route.go:42 |

### Needs an ADR
- <signal or gap>: <needs an ADR | fails "<criterion>">

### Notes
- <contradictions between docs and code, unknowns, questions>
```
