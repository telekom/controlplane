---
id: 15-prefer-shared-mocks
description: Tests must use the shared generated mocks instead of hand-written local doubles for the same interface.
severity: LOW
complexity: 1
scope: line
applies_to:
  - "**/*_test.go"
exclude:
  - "**/vendor/**"
context_lines: 30
---

# Prefer shared mocks

Shared, generated mocks already exist for the interfaces tests touch most:

- `common/pkg/client/fake` — `MockJanitorClient` (scoped / janitor client)
- `common/pkg/test/mock` — event recorder
- `secret-manager/api/fake` — `MockSecretManager`, `MockClientWithResponsesInterface`
- `file-manager/api/fake` — `MockFileManager`
- `<module>/test/mocks` and other `mock`/`fake` packages, e.g.
  `identity/test/mocks/keycloakclient`, `gateway/pkg/kong/client/mock`,
  `rover-server/test/mocks`

A hand-written double in a `_test.go` file for one of these interfaces
duplicates the shared mock, drifts when the interface changes, and hides
behavior from the next reader. Use the shared mock, and add expectations
(`EXPECT().…Run(…)`) for stateful behavior.

A local double is fine when no shared mock for that interface exists (e.g. a
small, package-internal interface), or when it wraps a real client to inject
a single failure (`struct{ client.Client }` overriding one method) in an
envtest. If a missing shared mock would be useful in several places,
generate one instead of writing a local one.

Judge only against the list above and imports visible in the context. If the
double's interface is not one of the listed shared mocks and no visible
import provides a mock for it, answer `PASS` — do not answer `UNCLEAR`
because a shared mock *might* exist elsewhere.

## Detect

```regex
^type\s+\w*(?i:mock|fake|stub|memory|inmemory|recording|failing|faulty|dummy)\w*\s+struct\b
```

## Checks

- [duplicates-shared-mock] Does the marked type implement an interface (secret manager, janitor/scoped client, file manager, recorder, Keycloak, Kong, or another interface with a mock in a `mock`/`fake`/`test/mocks` package) that already has a shared mock?

## Examples

### Violation

```go source=admin/internal/handler/zone/suite_test.go:207@ba54f7a6
type memorySecretManager struct {
	mu        sync.Mutex
	values    map[string]string
	published []string
	version   int
	failNext  int
	getErr    error
}
```

### Allowed

```go source=rover-server/internal/controller/apispecification_lint_test.go:32@ba54f7a6
type mockLinter struct {
	called  bool
	outcome oaslint.Outcome
	err     error
}
```

```go source=gateway/pkg/kongutil/client_factory_test.go:31@ba54f7a6
type failingReader struct{ client.Reader }
```
