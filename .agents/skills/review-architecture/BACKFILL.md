# Backfill

Backfilling turns decisions that already shape the code into ADRs. It is the
riskiest mode: the people who decided may be gone, the reasons may never have
been written down, and a fluent guess reads exactly like a fact. Every agent
after you will trust what you write, so a backfilled ADR claims only what its
sources show.

Work one domain (or one cross-cutting theme) per run. Each run ends in one
pull request that only a human can turn from `proposed` into `accepted`.

## 1. Harvest

Collect candidate decisions for the domain, with where each one shows up:

- The domain's page in `docs/docs/architecture/` and `overview.md`. Sentences
  that explain _why_ ("so that…", "instead of…", "because…") are the richest
  seam.
- `AGENTS.md`, `REVIEW.md` and `.agents/rules/`: every rule enforces a
  decision that may never have been recorded.
- Design specs and plans under `docs/superpowers/`.
- History: run `adr.py signals --mode range --base <c>~1 --revision <c>` over
  `git log --no-merges --format=%h -- <domain>` to find commits that added
  modules, cross-domain dependencies, libraries or API kinds. Read their pull
  requests with `gh pr view <n>`; the discussion often holds the reason.
- `go.mod`: third-party libraries that would take a quarter to replace.

Skip anything an accepted ADR already covers (`adr.py list`).

## 2. Triage

Write the candidates to a scratch file outside the repository, one row each:
candidate, evidence, the three criteria (hard to reverse, surprising without
context, real trade-off), and what is still unknown. Drop every row that
fails a criterion and say which one failed.

Show the table to the user and let them pick. Ask for the unknowns now, one
question at a time: above all, which alternatives were really considered, and
why they lost.

## 3. Draft

For each picked candidate:

1. `adr.py new "Title" --domain <dir> --backfilled`.
2. Write it as [ADR-FORMAT.md](./ADR-FORMAT.md) describes, with `evidence`
   listing every source you used.
3. Check each sentence of `Context`, `Decision` and `Options considered`:
   either a listed source says it, or the user told you. Delete or turn into
   a question anything else. An option no source and no person mentions does
   not go in, even when the validator then complains that alternatives are
   missing. That complaint is the question to ask. If the answer is that
   nothing else was weighed, write `No alternatives were evaluated.` as
   [ADR-FORMAT.md](./ADR-FORMAT.md) describes.
4. Check each claim against the code as it is today. Documentation goes
   stale, so a cited sentence can still be false. For every component the
   `Decision` names, read the code that would break the claim: its imports,
   what it creates or changes, and every `evidence` item. When code and
   documentation disagree, leave the claim out and ask which one is right.
5. Set `governs` to every file that can break the decision, not just where
   it is declared. A rule about module dependencies also binds the code that
   writes resources across that boundary.
6. `adr.py validate`.

## 4. Hand over

Open one pull request per domain with every ADR still `proposed`. In its
description, list per ADR the sentences that rest on the user's word rather
than a written source, and any question left open. The reviewer accepts,
corrects or rejects each record. Before a record turns `accepted`, its
claims are checked against the code again (step 4), because a reviewer's
approval of the pull request does not do that.
