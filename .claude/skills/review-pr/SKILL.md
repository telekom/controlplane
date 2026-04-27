<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

---
name: review-pr
description: Review a pull request covering technical (build, lint, coverage, error handling, security, API compat) and business (acceptance criteria, requirements, business logic, edge cases) dimensions. Use when asked to review a PR or evaluate code changes.
argument-hint: "[PR-number or empty for current branch]"
allowed-tools: Bash(gh *) Bash(git *) Read
---

Review the pull request specified by `$ARGUMENTS`.

If `$ARGUMENTS` is a PR number, use `gh pr view` and `gh pr diff` to fetch it.
If `$ARGUMENTS` is empty, review the current branch's diff against the base branch (main).

## Instructions

Read the project's `REVIEW.md` and `AGENTS.md` at the repo root first — they define the team's review conventions, coding standards, and what to skip.

Perform a thorough review covering **both technical and business dimensions**. Read every changed file in full before commenting on it — do not review based on the diff alone, because context matters.

### Phase 1: Gather context

1. Read `REVIEW.md` and `AGENTS.md` from the repository root.
2. If `$ARGUMENTS` is set, fetch PR metadata: `gh pr view $ARGUMENTS --json title,body,labels,author,baseRefName,headRefName,commits,url`. If `$ARGUMENTS` is empty, identify the branch context with `git branch --show-current` and compare against `origin/main`.
3. If `$ARGUMENTS` is set, fetch commit messages: `gh pr view $ARGUMENTS --json commits --jq '.commits[].messageHeadline'`. If `$ARGUMENTS` is empty, list commit messages with `git log --format=%s origin/main..HEAD`.
4. If `$ARGUMENTS` is set, fetch diff stat: `gh pr diff $ARGUMENTS --stat`. If `$ARGUMENTS` is empty, use `git diff --stat origin/main...HEAD`.
5. If `$ARGUMENTS` is set, fetch CI status: `gh pr checks $ARGUMENTS`. If `$ARGUMENTS` is empty, skip PR checks unless you first confirm a PR exists for the current branch; otherwise note that no PR-specific CI status is available.
6. Get the full diff: if `$ARGUMENTS` is set, run `gh pr diff $ARGUMENTS`; if `$ARGUMENTS` is empty, run `git diff origin/main...HEAD`. For large PRs or large branch diffs (>20k lines), prefer `git diff` with the branch ref instead.
7. Read each changed file in full (not just the diff hunks). Skip generated files per REVIEW.md.
8. If the PR description links to an issue, fetch the issue body for acceptance criteria.
9. Check the CI status — if CI already covers T3/T4/T6/T7, note the results instead of re-running locally.

### Phase 2: Technical review

Evaluate every item below. For each, state the finding with a severity marker and a one-line reason. Only expand into detail when the finding is non-trivial.

**Severity markers:**
- 🔴 **Important** — a bug or violation that should be fixed before merging
- 🟡 **Nit** — a minor issue, worth fixing but not blocking
- 🟣 **Pre-existing** — a problem that exists in the codebase but was NOT introduced by this PR
- ✅ **Pass** — check passes, no issues found

| #   | Check                       | How to verify                                                                                                                                                                          |
| --- | --------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| T1  | **Conventional commits**    | All commit messages match `<type>(<scope>): <description>`.                                                                                                                            |
| T2  | **REUSE / SPDX compliance** | Every new or modified file has a valid SPDX header (Apache-2.0 for code, CC0-1.0 for docs).                                                                                           |
| T3  | **Build**                   | `make build` succeeds in each affected module. Check CI first.                                                                                                                         |
| T4  | **Lint**                    | `make lint` passes in each affected module (uses root `.golangci.yml`; event and pubsub have their own). Check CI first.                                                               |
| T5  | **Tests exist**             | New or changed business logic has corresponding `*_test.go` files using Ginkgo/Gomega.                                                                                                |
| T6  | **Test coverage**           | Run `make test` in affected modules. Flag files with <60% coverage on changed lines. Check CI first.                                                                                   |
| T7  | **go mod tidy**             | `go mod tidy` produces no diff in any affected module. Check CI first.                                                                                                                 |
| T8  | **Generated code**          | If the module uses controller-gen (has `manifests`/`generate` make targets), verify generated files are up-to-date by running `make manifests generate` and checking for a clean diff.  |
| T9  | **Error handling**          | Uses domain error types (`ctrlerrors` for controllers, `problems.Problem` for REST). Errors wrapped with context, never swallowed.                                                     |
| T10 | **Security**                | No hardcoded secrets. JWT/LMS patterns followed. Auth failures use `problems.Forbidden()`. No injection vectors.                                                                       |
| T11 | **API compatibility**       | If CRD types or GraphQL schemas changed: Spec/Status separation, condition markers, `types.Object` interface, `ObservedGeneration` stamped.                                            |
| T12 | **Dependencies**            | New dependencies are justified. No unnecessary additions. Replace directives only reference local monorepo modules.                                                                    |

### Phase 3: Business review

| #  | Check                          | How to verify                                                                                                                                                  |
| -- | ------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| B1 | **Acceptance criteria**        | If the PR links to an issue with acceptance criteria, verify each criterion is addressed. List each criterion and whether it is met.                            |
| B2 | **Requirements coverage**      | The implementation matches what was requested — no under-delivery, no scope creep.                                                                             |
| B3 | **Business logic correctness** | Read the domain logic and verify it does what the PR description says. Flag anything that looks like a logic error, off-by-one, or incorrect state transition. |
| B4 | **Edge cases**                 | Identify at least 3 edge cases relevant to the change. State whether they are handled.                                                                         |
| B5 | **Naming and domain language** | New types, functions, and fields use language consistent with the existing domain model and AGENTS.md conventions.                                              |
| B6 | **Observability**              | Significant state changes or error paths use `logr` with structured key-value pairs. Events recorded via `recorder.Event()` where appropriate.                 |

### Phase 4: Output

Produce a structured review in this format:

```
## PR Review: <PR title>

### Summary
<2-3 sentence summary of what this PR does and the reviewer's overall impression>

### Technical
| Check | Status | Notes |
|-------|--------|-------|
| T1 Conventional commits | ✅/🔴/🟡/🟣 | ... |
| ... | ... | ... |

### Business
| Check | Status | Notes |
|-------|--------|-------|
| B1 Acceptance criteria | ✅/🔴/🟡/🟣 | ... |
| ... | ... | ... |

### Findings
<Numbered list of specific findings, ordered by severity (🔴 first, then 🟡, then 🟣).
Each finding should reference the file and line number.
Include a brief explanation of why it's an issue and a suggested fix.>

### Verdict
**APPROVE** / **REQUEST CHANGES** / **COMMENT**
<One-line justification>
```

Use **REQUEST CHANGES** if any finding is 🔴. Use **COMMENT** if there are only 🟡 or 🟣 findings. Use **APPROVE** only if everything is ✅.
