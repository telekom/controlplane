<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Code Review Instructions

Review for correctness and intent — coding conventions are documented in AGENTS.md and enforced by CI (golangci-lint, REUSE, tests). Don't duplicate those checks.

## Flag

- Double-logging: error returned from reconciler AND logged at the call site.
- Swallowed errors or partial success without returning an error (breaks requeue).
- Non-idempotent operations in reconcilers.
- External API calls without logging the response.
- Conditions set after status update, or missing `ObservedGeneration`.
- Security: JWT claims parsed manually instead of using middleware; missing LMS checks.
- New dependencies without justification.

## Verify

- Changed business logic has corresponding `*_test.go` coverage.
- CRD type changes: Spec/Status separation, standard condition list markers, `types.Object` implemented.
- `go.mod` replace directives only reference local monorepo modules.

## Skip

- Generated files: `zz_generated.deepcopy.go`, `*_generated.go`.
- `third_party/`, `builtin/`, `examples/`.
- Formatting — handled by golangci-lint and pre-commit hooks.
- Kubebuilder scaffolding boilerplate unless substantively modified.
