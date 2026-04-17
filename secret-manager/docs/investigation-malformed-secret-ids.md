<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Investigation: Malformed SecretIds in Application Onboarding (Conjur Backend)

**Date:** April 2026
**Scope:** Conjur backend, `clientSecret` during Application onboarding
**Status:** One bug fixed, remaining symptoms not reproducible

---

## Reported Symptoms

Three types of malformed `secretId` values were observed in Application onboarding responses:

1. **Missing checksum** — The secretId ends with an empty checksum segment, e.g. `poc:eni:foo:clientSecret:`.
2. **Unstable checksum** — The checksum changes between repeated onboarding requests before eventually stabilising.
3. **Data from other secrets** — The secretId contains fragments from unrelated secrets, e.g. `poc:eni:bar:blablabla`.

---

## Background

### SecretId format

Every secretId follows a five-part, colon-separated format:

```
env:team:app:path:checksum
```

The `checksum` is the first 12 hex characters of a SHA-256 hash computed from the secret **value**. When stored on resources, the secretId is wrapped in `$<...>` tags.

### How Application onboarding works

1. A Conjur policy is created for the application.
2. For each allowed secret (e.g. `clientSecret`, `externalSecrets`), the system generates a default value.
3. Each secret is written through the **CachedBackend** (Ristretto cache) wrapping the **ConjurBackend**.
4. If the secret already exists and its type does not allow changes, the stored value is kept and its checksum is returned.
5. After all secrets are written, any missing sub-path references are filled in by `MergeSecretRefs`.

### Concurrency controls

Two separate bouncers serialise access:

- **Onboard bouncer** — locks on the policy path (e.g. `controlplane/env/team`), serialising all onboarding for the same team.
- **Secret-write bouncer** — locks on the Conjur variable path (e.g. `controlplane/env/team/app/clientSecret`), serialising writes to the same variable.

---

## Investigation

### Full code review

The following areas were reviewed line by line:

| Area | Key files |
|---|---|
| Conjur onboarder | `pkg/backend/conjur/onboarder.go` |
| Conjur backend (read/write) | `pkg/backend/conjur/backend.go` |
| SecretId construction | `pkg/backend/conjur/id.go` |
| Cache layer | `pkg/backend/cache/backend.go` |
| Bouncer (distributed lock) | `pkg/backend/conjur/bouncer/lock.go` |
| Shared backend layer | `pkg/backend/secrets.go`, `secret_value.go`, `interface.go`, `util.go` |
| Controller and HTTP handler | `pkg/controller/onboard_controller.go`, `internal/handler/handler.go` |
| API client (consumer side) | `api/api.go` |
| Rover webhook (consumer) | `rover/internal/webhook/v1/secrets.go` |

### Code path tracing for `clientSecret`

Every code path that produces a `clientSecret` secretId was traced in isolation:

- **First creation** (secret does not exist): The backend writes the value and returns a checksum computed from that value. Correct.
- **Re-onboard** (secret already exists, `AllowChange=false`): The backend reads the stored value and returns a checksum computed from it. Correct.
- **User-provided value** (converted to `AllowChange=true`): The backend compares stored and incoming values and returns the appropriate checksum. Correct.

All isolated paths produce well-formed, correct checksums.

### Race detector

The Go race detector was run on all Conjur and cache tests. No races were detected.

### Conjur API client (conjur-api-go v0.13.19)

The third-party library was reviewed for thread safety:

- Each HTTP call creates its own `*http.Request` and receives its own `*http.Response`. Response bodies are read into fresh byte slices — there is no shared buffer that could cause data mixing.
- Token refresh (`RefreshToken`) has minor unsynchronised access to `authToken`, but this would cause authentication failures, not data corruption.
- The library is safe for our usage pattern because the bouncers serialise calls per variable.

---

## Bug Found and Fixed

### Stale checksum on no-op merge (`backend.go`, line 149)

**Location:** `pkg/backend/conjur/backend.go`, function `doSet`

**Problem:** When using the merge write strategy, the incoming JSON value is shallow-merged with the existing stored value. If the merged result equals the stored value (a no-op), the function returned the caller's original `id`, which carried a checksum computed from the **input** value, not the **effective** value after merge.

For example, if the stored value is `{"key1":"v1","key2":"v2"}` and the incoming value is `{"key1":"v1"}`, the merge produces the same as the stored value. However, the returned secretId would contain a checksum of `{"key1":"v1"}` instead of `{"key1":"v1","key2":"v2"}`.

**Fix:** The return statement now recomputes the checksum from the effective value:

```go
// Before (buggy)
return backend.NewDefaultSecret(id, currentValue), nil

// After (fixed)
return backend.NewDefaultSecret(id.CopyWithChecksum(backend.MakeChecksum(effectiveValue)), currentValue), nil
```

**Impact:** This fix primarily affects secrets using the merge strategy (e.g. `externalSecrets`). For `clientSecret` specifically, this path is less commonly hit because `clientSecret` does not typically use merge. However, it could contribute to unstable checksums (symptom 2) in edge cases.

---

## Tests Added

Three new tests were added to strengthen coverage of concurrent scenarios and the fix above.

### 1. Merge no-op checksum correctness

**File:** `pkg/backend/conjur/backend_test.go`

Verifies that when merge strategy produces a no-op (merged value equals stored value), the returned secretId carries the checksum of the stored value, not the input value.

### 2. Concurrent onboarding of different apps

**File:** `pkg/backend/conjur/onboarder_test.go`

Runs 20 goroutines, each onboarding a different application concurrently. Validates that every returned `clientSecret` secretId is well-formed (five colon-separated parts) with the correct `env`, `team`, `app`, and `path` segments and a non-empty checksum.

### 3. Repeated concurrent onboarding of the same app

**File:** `pkg/backend/conjur/onboarder_test.go`

Runs 10 goroutines across 3 rounds, all onboarding the same application. Validates that:
- All secretIds have non-empty checksums.
- After the initial creation round, all subsequent rounds return identical, stable checksums.

All tests pass with the Go race detector enabled.

---

## Unresolved: Symptom 3 (data from other secrets)

The most severe symptom — secretIds containing fragments from unrelated secrets — could not be reproduced through code analysis or concurrent testing.

Possible explanations include:

- **Infrastructure-level issues** such as a load balancer or proxy mixing HTTP response bodies between concurrent requests to the Conjur server.
- **A bug in an earlier version of the code** that has since been refactored away.
- **Misattribution** — the observed value may have been a different field or log entry, not the actual secretId.

### Recommended next steps

If symptom 3 reappears in a live environment:

1. **Add request-scoped tracing** — Log the full secretId at every construction point (`createSecrets`, `doSet`, `CachedBackend.Set`) with a correlation ID to trace which step produced the malformed value.
2. **Capture the raw Conjur API response** — Log the byte-level response from `RetrieveSecret` to rule out server-side data corruption.
3. **Check for HTTP-level issues** — Review load balancer and service mesh logs for response body interleaving or misrouted requests.

---

## Files Changed

| File | Change |
|---|---|
| `pkg/backend/conjur/backend.go` | Fixed checksum computation on no-op merge in `doSet` |
| `pkg/backend/conjur/backend_test.go` | Added merge no-op checksum test |
| `pkg/backend/conjur/onboarder_test.go` | Added two concurrent onboarding tests |
