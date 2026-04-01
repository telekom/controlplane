<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Secret-Manager Integration — Implementation Plan

> **Depends on:** [design.md](./design.md)
> **Estimated steps:** 3
> **Status:** Not started

---

## Step 1: Add `secret-manager` dependency to `go.mod`

**Goal:** Make the `secret-manager/api` package available in the spy-server module.

**Tasks:**
1. In `spy-server/go.mod`, add the direct require entry:
   ```
   github.com/telekom/controlplane/secret-manager v0.0.0
   ```
2. Add the corresponding replace directive:
   ```
   github.com/telekom/controlplane/secret-manager => ../secret-manager
   ```
3. Run `go mod tidy` inside `spy-server/` to ensure all transitive dependencies are recorded.

**Deliverables:**
- Updated `spy-server/go.mod` with the new dependency and replace directive.
- Clean `go mod tidy` (no missing or stale entries).

### 🚧 Gate 1: Module resolution
- [ ] `go mod tidy` completes without error
- [ ] `go build ./...` inside `spy-server/` passes
- [ ] Run **go-lint** skill

---

## Step 2: Wrap stores with `SecretStore` in `pkg/store/stores.go`

**Goal:** Resolve (or obfuscate) secret placeholders in `ApiSubscription` and `ApiExposure` CRDs before they reach the mapper layer.

**Tasks:**
1. Add imports in `spy-server/pkg/store/stores.go`:
   - `cconfig "github.com/telekom/controlplane/common/pkg/config"`
   - `"github.com/telekom/controlplane/common-server/pkg/store/secrets"`
   - `secretsapi "github.com/telekom/controlplane/secret-manager/api"`
2. Declare a package-level `secretsForKinds` map:
   ```go
   var secretsForKinds = map[string][]string{
       "ApiSubscription": {
           "spec.security.m2m.client.clientSecret",
           "spec.security.m2m.basic.password",
       },
       "ApiExposure": {
           "spec.security.m2m.externalIDP.client.clientSecret",
           "spec.security.m2m.externalIDP.basic.password",
           "spec.security.m2m.basic.password",
       },
   }
   ```
3. At the end of `NewStores`, after all base stores are initialised, add the conditional wrapping block:
   ```go
   if cconfig.FeatureSecretManager.IsEnabled() {
       secretsAPI := secretsapi.NewSecrets()
       resolver := secrets.NewSecretManagerResolver(secretsAPI)

       s.APISubscriptionStore = secrets.WrapStore(
           s.APISubscriptionStore,
           secretsForKinds["ApiSubscription"],
           resolver,
       )
       s.APIExposureStore = secrets.WrapStore(
           s.APIExposureStore,
           secretsForKinds["ApiExposure"],
           resolver,
       )
   }
   ```

**Deliverables:**
- Updated `spy-server/pkg/store/stores.go`.

### 🚧 Gate 2: Build & lint
- [ ] `go build ./...` inside `spy-server/` passes
- [ ] Run **go-lint** skill

---

## Step 3: Add unit tests for secret resolution and obfuscation

**Goal:** Verify that the `secretsForKinds` paths are correct and that `SecretStore` correctly resolves and obfuscates secret fields when called through the spy-server stores.

**Tasks:**

1. Create `spy-server/pkg/store/stores_secret_test.go` (or a new `suite_test.go` in a new package `store_test`).
2. For each wrapped kind (`ApiSubscription`, `ApiExposure`), write a table-driven test using `t.Run` subtests:
   - **resolve**: inject a CRD whose secret field contains a `$<fake-id>` placeholder; wrap with a test `Replacer` that returns `"resolved-value"`; assert the returned object's field equals `"resolved-value"`.
   - **obfuscate**: inject a context where `security.IsObfuscated` returns `true`; assert the returned object's field equals `**********`.
   - **no-op when feature disabled**: bypass wrapping entirely; assert the placeholder is returned unchanged.
3. Use `common-server/pkg/store/secrets.WrapStore` directly in tests (not `NewStores`) to avoid needing a real k8s cluster or secret-manager.

**Deliverables:**
- New test file(s) covering resolve, obfuscate, and feature-disabled paths.

### 🚧 Gate 3: Tests pass & coverage
- [ ] All new tests pass (`go test ./pkg/store/...`)
- [ ] Run **go-test-coverage** skill (target: ≥ 80% for `spy-server/pkg/store`)
- [ ] Run **go-lint** skill

---

## Summary

| Step | Description | Depends On | Gate |
|------|-------------|------------|------|
| 1 | Add `secret-manager` dependency | — | Build passes |
| 2 | Wrap stores in `pkg/store/stores.go` | Step 1 | Build + lint |
| 3 | Unit tests for resolution & obfuscation | Step 2 | Tests + coverage + lint |

**Parallelism:** Steps 1 and 3 (skeleton) can be drafted in parallel, but Step 3 requires Step 2 for the actual implementation.
