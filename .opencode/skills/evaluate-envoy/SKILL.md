---
name: evaluate-envoy
description: Use when checking whether Envoy satisfies the gateway requirements in .opencode/agent/strict-reviewer/requirements.md. Runs an interview loop between the strict-reviewer and envoy-expert subagents to reach a cited verdict per requirement. Trigger on "evaluate envoy", "does envoy support", "check envoy requirements".
---

# Evaluate Envoy

This is a **back-and-forth interview loop between two subagents** —
`strict-reviewer` (the interviewer) and `envoy-expert` (the interviewee). They
question and answer each other repeatedly until a verdict is reached; a single
question-and-answer is NOT enough. Because subagents cannot call each other,
YOU are the relay: carry each message between them, turn by turn, keeping the
interview going until the reviewer is satisfied or the follow-up limit is hit.
Stay concise; every claim needs a citation.

## Inputs
- Requirements: `./.opencode/agent/strict-reviewer/requirements.md` (IDs like RT-01, AU-03).
- Scope: evaluate all requirements unless the user names specific IDs.

## Loop (per requirement ID)
Keep relaying between the two subagents until the reviewer settles the ID. The
review→follow-up→answer exchange is the interview — repeat it, don't shortcut it.

1. **Ask** — `strict-reviewer` (Task tool) states the acceptance question for the ID.
2. **Answer** — pass that question to `envoy-expert` (Task tool). It replies with a cited claim.
3. **Review** — pass the answer back to `strict-reviewer`. It returns one of:
   - `MET` — evidence sufficient.
   - `NOT MET` — Envoy lacks it.
   - `FOLLOW-UP: <question>` — gap or missing citation; relay to `envoy-expert` and repeat step 3.
4. **Stop** at `MET`/`NOT MET`, or after **3 follow-ups** (then record `INCONCLUSIVE`).

Batch independent requirements: fan out multiple Task calls in parallel where
IDs don't depend on each other.

## Output
One row per requirement, nothing else:

```
| ID | Verdict | Evidence (citation) | Notes |
```

Verdict ∈ MET / PARTIAL / NOT MET / INCONCLUSIVE. Every non-NOT-MET row must
carry a citation (URL or doc section). End with a one-line count summary.

No prose beyond the table and the summary line.
