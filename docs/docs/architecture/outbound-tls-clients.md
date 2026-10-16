---
sidebar_position: 25
---

<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Outbound TLS Clients

Admin, Gateway, and Identity share the HTTP client-building machinery for custom CA trust. This page describes its boundaries and lifecycle for contributors. For resource examples, rotation, and recovery, see [Custom CA Certificates](../admin-journey/environments-and-zones.md#custom-ca-certificates).

## Independent module boundaries

The two shared Go modules remain independent: **neither `common` nor `common-server` imports the other**.

| Layer | Responsibility |
| --- | --- |
| `common/pkg/types` | The optional TLS configuration and required ConfigMap name/key reference. |
| `common/pkg/cabundle` | Namespace-local ConfigMap reads, CA validation, deduplication, deterministic PEM encoding, and size limits. |
| `common-server/pkg/client/tlsclient` | System-plus-custom trust pools, HTTP transports, OAuth, client fingerprints, leases, and cache cleanup. |
| Component adapters | Supply the cached Kubernetes client, select owner identity and HTTP settings, resolve credentials, and translate errors into controller semantics. |

The bridge is a small interface owned by `tlsclient`: `Resolve(ctx context.Context) ([]byte, error)`. It returns already validated canonical PEM; an empty result means system roots only. The common resolver satisfies this interface without importing it. Semantic CA validation stays in `common`, rather than being duplicated in the HTTP layer. A configured reference without a usable resolver is an error, not permission to ignore custom trust.

The HTTP layer extends the system certificate pool and fails if loading it fails. It does not modify the existing internal CA refresher or other shared HTTP-client helpers. Token and API requests use the same additive trust, with certificate and hostname verification enabled for operators.

### Validation limitation

The intended source contract allows only CA certificate blocks and whitespace. The current `cabundle.parseSet` checks the input prefix and then delegates to Go's `pem.Decode`, which can skip a malformed block or unmatched `BEGIN` line before a later valid block. Such mixed input is therefore not guaranteed to be rejected, despite the strict contract. Source files must be clean CA exports; the parser is not a sanitization boundary. This is an implementation gap, not support for mixed PEM content.

## Component profiles and callers

| Component | Actual remote callers | Authentication and HTTP behavior |
| --- | --- | --- |
| Admin | OIDC discovery during Zone status population | Unauthenticated, 10-second timeout, cloned default transport with environment proxy support. Uses an uncached client with the validated in-memory Zone union and closes it after discovery. Existing discovery-result reuse remains. |
| Gateway | Route and Consumer create/update/delete handlers | Client credentials; token URL remains the issuer plus `/protocol/openid-connect/token`. Raw transport, 10-second timeout, 100 maximum idle connections per host, existing Gateway metrics. |
| Identity | Realm and Client create/update/delete handlers, including realm issuer discovery | Initial password grant is eager, followed by token refresh. Raw transport, TLS 1.3 minimum, 10-second timeout or `CLIENT_TIMEOUT`, existing Identity metrics. Issuer discovery shares the TLS transport and timeout without sending OAuth credentials. |

Gateway and Identity each construct **one factory per operator process**, register it with the manager, and inject it across their remote-calling controllers. The Gateway and IdentityProvider handlers do not gain proactive CA validation or connectivity probes; failures are reported on the operation resource.

## Acquisition, cache isolation, and leases

Each acquisition follows this sequence:

1. Resolve and validate CA material through the manager's cached client, using the acquisition context. Do this **before every cache lookup**, including a likely hit. A resolution failure must not return an older cached client.
2. Identify the owner by resource kind, namespace, and UID. Gateway uses its own identity; Identity uses the IDP identity recorded in the Realm connection snapshot.
3. Fingerprint the immutable HTTP profile, API and token endpoints, authentication mode, resolved credentials, and canonical CA bytes. The profile name identifies settings registered once for that factory.
4. Reuse or construct the client for that owner **and configuration version**, then return a lease.

Concurrent acquisitions of the same version share construction. Distinct owners do not share clients, even with identical endpoints and credentials. Realms and Clients using the same IDP and connection settings can share one client; old and new Realm connection snapshots can coexist as separate versions. Failed construction is removed from the cache so a later acquisition can try again. Rotation creates a new transport and trust pool rather than mutating one already in use.

Handlers defer the returned release function immediately after successful acquisition, on create/update and deletion paths alike. Keep a lease only for the short operation; do not retain it between reconciliations or use its client after release. Callers release leased clients rather than closing them.

One cleanup loop per factory checks for unused clients every minute. A version is eligible for eviction after **30 minutes since its last acquisition**, and only with zero borrowers. Eviction cancels that client's token-source context and closes the underlying transport's idle connections, even when OAuth and metrics wrap it. Manager shutdown closes the factory and its clients; cleanup runs on every replica without requiring leadership.

## Context and authentication lifetime

Cached token sources use the **operator lifetime**, never a reconciliation context retained from client construction. Each token HTTP request has the profile's timeout; there is no single construction deadline that expires the shared token source later. API requests still carry the current reconciliation context. A canceled caller may have to wait for a shared, timeout-bounded token refresh; there is no separate context-aware token broker.

Identity falls back to a password grant only when refresh returns HTTP 400 or 401. Transient refresh failures do not trigger password re-authentication. The Identity-supplied error classifier retains existing retry behavior, including the delay for HTTP 429 and retries for 5xx responses. Initial password-grant failures remain wrapped acquisition errors rather than passing through that refresh classifier.

## Kubernetes consistency and error semantics

All feature reads use the **manager cache**, not an API reader or the environment-scoped helper that rejects unlabeled objects. Admin, Gateway, and Identity need ConfigMap `get/list/watch`; Admin additionally writes managed bundles. Their ConfigMap caches are not restricted to platform-labeled objects. Validation on acquisition means validation of the latest value visible in that cache, not a strongly consistent read from the API server.

Admin indexes Zone source references by ConfigMap name in the Zone namespace. This index is registered independently of webhooks. Source events enqueue referencing Zones, regardless of which key changed; managed-bundle events map back through management labels for repair. There are no CA-to-Route/Consumer/Realm/Client fanout watches. Downstream recovery uses normal reconciliation and jittered retries.

Invalid CA content, absent ConfigMaps/keys, and forbidden source reads are configuration errors: Admin and Gateway return blocked controller errors, and Identity uses an equivalent blocked-error interface. Transient Kubernetes failures are retried (Admin explicitly classifies them as retryable; Gateway and Identity use the normal error/backoff path). Resolver errors remain wrapped through the HTTP factory, which has no dependency on controller error types. Remote TLS, authentication, and API failures remain operation failures, not evidence that a bundle should be rolled back.

## Realm snapshots and deletion

Alongside its existing admin connection fields, a Realm records `status.connection.identityProvider` with the IDP name, namespace, and UID. Optional `status.connection.tls` records the source ConfigMap name/key and **originating IDP namespace**. It stores no CA PEM. Normal reconciliation populates this metadata together with the other connection fields; deletion can use the snapshot without a live IDP lookup. Clients use their Realm's snapshot.

Compatibility is deliberately narrow:

- No `status.connection`: legacy system-only trust and cache isolation by the consuming Realm's namespace/UID.
- Present connection metadata: IDP name, namespace, and UID are required. Present TLS metadata must include namespace, ConfigMap name, and key. Incomplete metadata blocks the operation; it never silently drops TLS.
- Removing IDP TLS clears the TLS reference on subsequent Realm reconciliation. Pending snapshots keep the Zone bundle retained until they stop referencing it.

A missing trust source prevents required remote deletion and leaves the finalizer in place. Existing deletion exceptions are unchanged, notably Client deletion skipping the remote call when the Realm is absent or not ready. There is no new TLS bypass for deletion.

## Zone publication boundary

Admin validates the full source union before publishing `zone-ca-bundle/ca-bundle.pem` in the generated namespace or changing child connection settings. It publishes the bundle first; later steps are not transactional and do not roll it back. Invalid input preserves the last valid publication and children, while externally damaged managed output can still cause acquisition failures until repaired.

Management uses the Admin domain, environment, and Zone-name labels, not a cross-namespace owner reference. This protects unmanaged collisions and permits a same-named recreated Zone to adopt a correctly labeled retained bundle. Removing all sources clears references from retained managed children, including removed instances. Retirement waits for Gateway/IDP references in the zone namespace and Realm snapshots cluster-wide, using cached lists. Zone deletion itself retains the bundle and children.
