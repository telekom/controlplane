<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Secret-Manager Integration — Design Document

> **Version:** 1.0
> **Date:** 2026-04-01
> **Status:** Draft
> **Component:** `spy-server/`
> **Depends on:** [000_initial design](../000_initial/design.md), [002_event design](../002_event/design.md)
> **Reference Implementation:** `rover-server/pkg/store/stores.go`, `common-server/pkg/store/secrets/`

---

## Table of Contents

1. [Overview](#1-overview)
2. [Scope](#2-scope)
3. [Architecture](#3-architecture)
4. [Layer-by-Layer Design](#4-layer-by-layer-design)
   - [4.1 Secret Placeholders in CRDs](#41-secret-placeholders-in-crds)
   - [4.2 Store Layer — SecretStore Wrapper](#42-store-layer--secretstore-wrapper)
   - [4.3 Obfuscation](#43-obfuscation)
   - [4.4 Feature Flag](#44-feature-flag)
5. [Secret Fields per Resource Kind](#5-secret-fields-per-resource-kind)
6. [Changes Required](#6-changes-required)
7. [Security Model](#7-security-model)
8. [Potential Issues & Risks](#8-potential-issues--risks)
9. [Decision Log](#9-decision-log)

---

## 1. Overview

The CRDs watched by spy-server (`ApiExposure`, `ApiSubscription`) can contain **secret placeholders** of the form `$<secret-id>` in sensitive fields (e.g. `clientSecret`, `password`). These placeholders are managed by the **secret-manager** service. Without resolution, the spy-server returns raw placeholders to clients, which is incorrect.

This design integrates the secret-manager client library (`secret-manager/api`) into spy-server at the **store layer**, following the same pattern used in `rover-server`. When a request is served:

- If the caller has a **read or read-write** scope → secret placeholders are resolved to their actual values via the secret-manager API.
- If the caller has an **obfuscated** scope → secret fields are masked with `**********`.
- If `FeatureSecretManager` is **disabled** → stores pass through raw CRD data unchanged (no wrapping).

The `Application` resource in spy-server does **not** expose any secret-bearing field in its API response (`status.clientSecret` is not mapped), so its store does **not** require wrapping.

### Key Facts

| Aspect | Value |
|--------|-------|
| Integration point | `spy-server/pkg/store/stores.go` |
| Library used | `github.com/telekom/controlplane/secret-manager/api` |
| Store wrapper | `common-server/pkg/store/secrets.WrapStore` |
| Feature flag | `common/pkg/config.FeatureSecretManager` (default: `true`) |
| Resolver | `secrets.NewDefaultSecretManagerResolver()` |
| Obfuscation | `secrets.NewObfuscator()` (inside `SecretStore`) — automatic |
| Resources requiring wrapping | `ApiSubscription`, `ApiExposure` |
| Resources NOT requiring wrapping | `Application`, `Zone`, `Approval`, `EventExposure`, `EventSubscription`, `EventType` |

---

## 2. Scope

### In Scope

- Wrap `APISubscriptionStore` and `APIExposureStore` with `secrets.SecretStore` when `FeatureSecretManager` is enabled.
- Declare `secretsForKinds` map with the correct JSON paths for each kind.
- Add the `secret-manager` module dependency to `go.mod`.
- Unit tests verifying that secret placeholders are resolved (and obfuscated) correctly.

### Out of Scope (Deferred)

- Wrapping `ApplicationStore` — `status.clientSecret` is not returned by the spy-server Application API response.
- Wrapping `EventSubscription`, `EventExposure`, `EventType` — these CRDs contain no secret fields.
- Wrapping `ZoneStore` or `ApprovalStore` — no secret fields.
- Changes to controllers, mappers, or the server layer — the wrapping is transparent to all callers.

---

## 3. Architecture

### 3.1 Data Flow (with secret resolution)

```
HTTP Request
    │
    ▼
┌──────────────────────────┐
│   Fiber Middleware        │  ← JWT → BusinessContext (AccessType: read | obfuscated | all)
└────────────┬─────────────┘
             │
             ▼
┌──────────────────────────┐
│   Server Layer            │  (unchanged)
└────────────┬─────────────┘
             │
             ▼
┌──────────────────────────┐
│   Controller Layer        │  (unchanged — calls stores.APISubscriptionStore / APIExposureStore)
└────────────┬─────────────┘
             │
             ▼
┌──────────────────────────────────────────────────────────┐
│   SecretStore[T] (common-server/pkg/store/secrets)        │
│                                                           │
│   Get(ctx, ns, name):                                     │
│     1. Delegate to wrapped ObjectStore (inmemory)         │
│     2. If security.IsObfuscated(ctx) → Obfuscator        │
│        else                           → SecretResolver    │
│     3. ReplaceAll(ctx, obj, jsonPaths) → resolved object  │
└────────────────────┬─────────────────────────────────────┘
                     │
          ┌──────────┴──────────┐
          ▼                     ▼
┌──────────────────┐   ┌─────────────────────────┐
│  InMemory Store   │   │  SecretManagerResolver   │  ← secret-manager svc
│  (raw CRD data)   │   │  (resolves $<id> tokens) │    via SA JWT token
└──────────────────┘   └─────────────────────────┘
```

### 3.2 Key Differences from rover-server

| Aspect | rover-server | spy-server |
|--------|-------------|------------|
| Plain store kept separately? | Yes (`RoverStore` + `RoverSecretStore`) | No — same store field is replaced in-place |
| Reason for keeping both | Needs plain store for write mutations | Read-only; one store per kind is sufficient |
| Kinds wrapped | `Rover`, `Application` | `ApiSubscription`, `ApiExposure` |
| Feature-gated wrapping | No (always wraps) | Yes — wraps only if `FeatureSecretManager.IsEnabled()` |

---

## 4. Layer-by-Layer Design

### 4.1 Secret Placeholders in CRDs

The secret-manager stores secret values keyed by a secret ID. When a controller writes a credential to a CRD, it stores a placeholder instead of the raw value:

```
spec.security.m2m.client.clientSecret = "$<env/team/app/clientSecret>"
```

The `SecretManagerResolver` detects the `$<…>` prefix/suffix and calls `secrets.Get(ctx, secretID)` to obtain the real value before it is returned to the caller.

### 4.2 Store Layer — SecretStore Wrapper

`common-server/pkg/store/secrets.WrapStore` produces a `SecretStore[T]` that wraps any `store.ObjectStore[T]`. On every `Get` and `List` call it applies `Replacer.ReplaceAll(ctx, obj, jsonPaths)` to the returned objects.

**No changes to controllers or mappers are needed** — the wrapping is transparent.

In `spy-server/pkg/store/stores.go`, after all base stores are created, the relevant stores are conditionally replaced:

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

Because spy-server is **read-only**, there is no need to retain a plain (unwrapped) store alongside the secret store. The single store field per kind is replaced in-place, keeping the `Stores` struct and all controllers unchanged.

### 4.3 Obfuscation

`SecretStore` internally holds two `Replacer` implementations:

| Replacer | Behaviour |
|----------|-----------|
| `SecretManagerResolver` | Calls secret-manager API to fetch and inject real values |
| `Obfuscator` | Replaces each secret JSON path with `**********` |

The choice is made at request time via `security.IsObfuscated(ctx)` (reads `AccessType` from the JWT-derived `BusinessContext`). This logic lives entirely within `SecretStore`; neither controllers nor mappers need to be aware.

### 4.4 Feature Flag

`cconfig.FeatureSecretManager` is defined in `common/pkg/config/feature.go` as:

```go
FeatureSecretManager Feature = NewFeature("secret_manager", true)
```

It defaults to `true`. Operators can disable it via the config key `feature-secret_manager-enabled: false` (e.g. for local development without a running secret-manager).

When disabled, stores are not wrapped and all CRD data is returned verbatim (placeholders visible). This is the current behaviour of spy-server before this integration.

---

## 5. Secret Fields per Resource Kind

### ApiSubscription (`api.cp.ei.telekom.de/v1`)

Sensitive fields in `spec.security.m2m`:

| JSON Path | Field |
|-----------|-------|
| `spec.security.m2m.client.clientSecret` | OAuth2 client secret |
| `spec.security.m2m.basic.password` | Basic auth password |

### ApiExposure (`api.cp.ei.telekom.de/v1`)

Sensitive fields in `spec.security.m2m`:

| JSON Path | Field |
|-----------|-------|
| `spec.security.m2m.externalIDP.client.clientSecret` | External IdP OAuth2 client secret |
| `spec.security.m2m.externalIDP.basic.password` | External IdP basic auth password |
| `spec.security.m2m.basic.password` | Provider-side basic auth password |

---

## 6. Changes Required

| File | Change |
|------|--------|
| `spy-server/go.mod` | Add `github.com/telekom/controlplane/secret-manager v0.0.0` + replace directive |
| `spy-server/pkg/store/stores.go` | Add `secretsForKinds` map; conditionally wrap `APISubscriptionStore` and `APIExposureStore` |
| No other files | Controllers and mappers are unaffected |

---

## 7. Security Model

The secret-manager client (`secretsapi.NewSecrets()`) uses the **service-account JWT** mounted at `/var/run/secrets/secretmgr/token` when running in-cluster, or the `SECRET_MANAGER_TOKEN` environment variable locally. The spy-server pod must have:

1. A `ServiceAccount` with an audience of `secret-manager`.
2. A mounted `projected` volume for the SA token with the custom audience.
3. Network access to `https://secret-manager.controlplane-system.svc.cluster.local/api`.

This is identical to how rover-server is deployed.

---

## 8. Potential Issues & Risks

| # | Issue | Severity | Mitigation |
|---|-------|----------|------------|
| 1 | Secret-manager unavailable during startup | Medium | `SecretStore` resolves lazily on each request; startup itself is unaffected. Requests will fail with an error until the secret-manager is reachable. |
| 2 | Performance overhead per request | Low | Secrets are resolved per Get/List call. The secret-manager client should be evaluated for caching. Consider adding a TTL cache in a follow-up if latency is an issue. |
| 3 | Feature flag causes silent placeholder exposure | Low | When disabled, callers with write rights can see raw `$<…>` placeholders. This is acceptable for local/dev environments only; production always runs with the flag enabled. |

---

## 9. Decision Log

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | Replace store in-place rather than adding a separate `SecretStore` field | spy-server is read-only; there is no need to keep a plain store for writes. Keeping the `Stores` struct unchanged minimises diff. |
| 2 | Do not wrap `ApplicationStore` | The spy-server `ApplicationResponse` does not expose `status.clientSecret`; wrapping would resolve secrets that are never returned. |
| 3 | Do not wrap event resource stores | `EventExposure`, `EventSubscription`, and `EventType` CRDs contain no secret-bearing fields. |
| 4 | Gate wrapping behind `FeatureSecretManager` | Allows running spy-server without a secret-manager (e.g. in CI/local dev) by setting `feature-secret_manager-enabled: false`. |
| 5 | Use `secrets.NewDefaultSecretManagerResolver()` | Delegates URL, token, and TLS configuration to the library defaults, consistent with rover-server. |
