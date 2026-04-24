<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Controlplane Monorepo

Go monorepo for Kubernetes controllers and REST APIs. Each component has its own `go.mod` with local `replace` directives.

## Build & Test

Always use Makefile targets — never run `go build`, `go test`, `go vet`, or `golangci-lint` directly. The Makefiles ensure correct flags, code generation, coverage config, and envtest setup.

```bash
# Per-module (from module directory)
make build          # build (includes generate, fmt, vet where applicable)
make test           # test (includes build deps + envtest + coverage)
make lint           # golangci-lint (not all modules have this yet)

# Repo-wide (from repo root)
make verify         # all pre-push checks: pre-commit, tidy, build, lint
make build-all      # build every module
make test-all       # test every module
make lint-all       # lint every module that has a lint target
```

## Error Handling

### Controllers (Kubernetes reconcilers)
- Use domain error types from `common/pkg/errors/ctrlerrors`: `BlockedErrorf`, `RetryableErrorf`, `RetryableWithDelayErrorf`.
- If kubebuilder will log a returned error, don't log it again — no double-logging.
- Partial success (e.g. created 2 of 3 resources) must return an error so the object is requeued.
- Write idempotent operations — reconcilers can be called at any time.
- When calling external APIs (gateway, identity, etc.), log the response and record relevant details as events.

### REST/HTTP handlers
- Return `problems.Problem` (RFC 9457) for all error responses — not raw status codes or ad-hoc JSON.
- Use the helpers: `NotFound()`, `Forbidden()`, `BadRequest()`, `InternalServerError()`, etc.

### General
- Wrap errors with context: `fmt.Errorf("doing X: %w", err)`.
- Never swallow errors silently or assign to `_`.
- For obvious developer errors use the "...orDie" approach — and cover with a test.

## Logging

Uses `logr` backed by Zap. Get the logger from context: `logr.FromContextOrDiscard(ctx)`.

- **V(0)** = info: system state changes, operational activity ("created consumer", "configuring feature X").
- **V(1)** = debug: detailed troubleshooting, step-by-step leading to errors.
- There is no warn level — decide if it's info or error.
- Use constant messages with structured key-value pairs:
  `log.V(1).Info("Created ACL plugin for route", "aclPluginId", pluginId, "routeName", route.Name)`
- Don't log confidential data.
- Don't duplicate context already available (namespace, name are in the logger context).

## Reconciliation Flow

Standard controller flow: fetch -> first setup (finalizer, init conditions) -> validate environment -> handle deletion or reconcile -> update status.

- Set conditions **before** the status update, not after.
- Stamp `ObservedGeneration` on all conditions.
- Requeue with jitter — never fixed intervals.
- Use the `Handler[T]` interface with `CreateOrUpdate` and `Delete` methods.

## Testing

- Use **Ginkgo v2 / Gomega** — not testify or stdlib assertions.
- Use **envtest** for Kubernetes API tests — not mocks.
- Use `Eventually()` with explicit timeout and interval for async assertions.
- New or changed business logic must have `*_test.go` files.

## Documentation

User-facing documentation lives in `docs/docs/` (Docusaurus). After any change to behavior, APIs, CRDs, or configuration, check whether the relevant docs need updating:

- `user-journey/` — end-user workflows (exposing/subscribing to APIs and events, approvals, applications)
- `admin-journey/` — operator workflows (installation, environments, organizations, features)
- `developer-journey/` — contributor workflows (local development, creating operators)
- `architecture/` — per-component design docs
- `reference/` — API reference, JSON schemas

Update docs in the same PR as the code change.

## Conventions

- Commit messages: Conventional Commits `<type>(<scope>): <description>`.
- New files: SPDX license header (`Apache-2.0` for code, `CC0-1.0` for docs).
- Prefer early returns over nested conditionals.

