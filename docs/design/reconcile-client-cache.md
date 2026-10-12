<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Shared CA and reconcile-scoped client caches

**Status: approved and implemented. `make test` passes in common, common-server, gateway, identity and admin. Root `make verify` for these modules remains blocked by 281 `common-server` lint findings outside the changed provider files. Both caches live in `common/pkg/providerca`; `common-server` keeps the concrete PEM, TLS and transport helpers and does not depend on `common`.**

## Eventual-consistency simplification (supersedes conflicting statements below)

The following decisions override stronger guarantees described elsewhere in this document:

- **Cache-only Kubernetes reads.** Gateway and Identity resolve CA references through the shared CA cache wired to
  `mgr.GetClient()`, i.e. the informer cache; there is no APIReader or direct-read fallback. The ConfigMap informer is
  label-filtered (`cp.ei.telekom.de/provider-ca=true`), and the existing contract already requires that label on
  every referenced source and copy. A referenced ConfigMap without the label is therefore invisible to the cache and
  reported as "not found or lacks label" (Gateway reason `ConfigMapNotFound`); it never falls back to system trust.
  Admin also uses the informer cache for sources, bundle copies, Zone liveness and published children.
  It validates copies from write responses without reading them back. Stale reads causing
  `AlreadyExists` or write conflicts are retried. Admin no longer lists Realms to gate target moves.
- **No IdentityProvider revalidation, no early snapshot commit.** The Realm handler derives its connection from the
  (cached) IdentityProvider, records it in `Realm.status` in memory and calls the factory with that Realm. The normal
  controller status update persists it. The factory never rereads the Realm.
- **No target freezing.** Identity and Admin no longer reject IdentityProvider target moves for Realms with a recorded
  connection; partial connections are completed from the IdentityProvider.
- **Client replacement.** The shared client cache still fingerprints owner UID, endpoints, credentials and CA
  reference/UID/digest and replaces the client (new transport, cloned TLS config) on change; replaced clients close
  idle connections only, so in-flight operations finish on the previous client. There is no borrow/release or
  reference counting.

Accepted limitations: configuration and trust can be temporarily stale (informer lag, one-minute CA cache);
a crash between a remote request and the status update can leave the recorded connection behind the remote state and
cleanup incomplete; endpoint changes are not a managed migration and may leave remote resources at the old endpoint.
No consistency-transaction machinery is reintroduced to address these.

## 1. Goal

Simplify provider CA support by moving ConfigMap handling into one CA cache and
client reuse/replacement into one shared typed client cache, both in
`common/pkg/providerca`. Gateway and Identity retain only their provider-specific
connection extraction, validation, error mapping and client construction.

This replaces the duplicated machinery; it must not become another layer over
the existing factories' caches.

### Hard acceptance criterion: shared functionality, minimal provider code

Shared ownership is the primary requirement, not an optional cleanup. Gateway
and Identity must not retain independent implementations of CA resolution or
client-cache behavior behind forwarding wrappers.

The shared modules own ConfigMap reads and validation, parsing orchestration,
CA caching/expiry, connection fingerprinting, client caching/replacement,
concurrency, eviction, transport retirement and cache lifecycle. Their tests
exercise those contracts once, rather than duplicating the same cache test
suite for each provider. Provider status validation uses the same injected CA
cache as acquisition.

Provider-local code is limited to extracting connection/credential inputs,
selecting the authentication flow and constructing the provider client, mapping
shared errors to provider conditions, and wiring dependencies. Existing domain
logic such as Realm cleanup snapshots remains domain logic, not a second cache.
Preserving provider-specific TLS/timeouts does not justify duplicating transport
construction or lifetime management.

Implementation is not complete if both original factories still contain their
own cache maps, fingerprints, freshness checks, invalidation protocols or
lifecycle bookkeeping. Remove superseded code and hooks; do not merely route
some calls through the shared implementation.

If a current-code constraint would require substantial CA/cache behavior to
remain in Gateway or Identity, stop and report the concrete constraint and
options to the user. Do not silently weaken this requirement, introduce a
parallel implementation, or add a compatibility layer that preserves the
duplicated machinery.

### Agreed requirements

- A Zone references source CA ConfigMaps.
- The Zone reconciler combines them into one CA bundle in the zone namespace.
- Every Gateway and IdP of the Zone receives the same reference to that bundle.
- Shared reference structs and the CA cache live in `common`.
- The client cache also lives in `common` and receives the CA cache by injection.
- `common-server` keeps the strict PEM parser, bundle, TLS and transport helpers and
  does not import `common`. Providers inject the parser and their builders.
- Gateway and Identity use the same client-cache implementation.
- Clients acquired from the cache are used only for the span of a reconciliation.
- Replacement is eventually consistent, not immediate.
- If a required refresh fails, acquisition returns an error and reconciliation
  retries later. There is no stale-trust fallback.
- An explicit CA reference must never silently fall back to system-only trust.

### Non-goals

- Immediate revocation or invalidation of clients already borrowed.
- Background refresh workers.
- Client leases, borrower reference counting or a retired-client registry.
- Epochs, repeated authoritative confirmation reads or publication retry loops.
- A general-purpose configurable caching framework.
- CRD or endpoint-helper redesign.

## 2. Architecture and ownership

```text
Zone source ConfigMap references
  |
  v
Zone reconciler merges them into one Zone CA bundle in the zone namespace
  |
  v
every Gateway / IdP of the Zone references that bundle

common/pkg/providerca
  +-- existing shared CA-reference structs and resolver
  +-- CA cache: ConfigMap lookup, validation, parsing and TTL refresh
  |     (parser injected from common-server by the provider wiring)
  +-- shared typed client cache: reuse, replacement and lifecycle
        +-- Kong-specific builder (gateway)
        +-- Keycloak-specific builder (identity)

common-server/pkg/client
  +-- strict PEM parser, ProviderCABundle, TLS config and provider transports
```

Caches are process-local. Construct them once per manager and share them among
that manager's controllers. Gateway and Identity use the same implementation,
not a cross-process cache or package-global singleton.

## 3. CA cache in common

Add `common/pkg/providerca/cache.go` alongside the existing resolver.

Reuse:

- `common/pkg/types/providertls.go`: `CABundleConfigMapRef` and `ProviderTLS`.
- `common/pkg/providerca/resolve.go`: reference, label, provenance and selected-key
  validation, structured errors, and parser-injected resolution.
- `common-server/pkg/client/provider_tls.go`: strict PEM parser and parsed bundle.

Keep parsing implementation in `common-server` and inject
`ParseProviderCABundle` when constructing the CA cache in provider wiring. The
common cache owns when parsing happens and caches the result without importing
`common-server`.

### Illustrative interface

```go
func NewCache[B any](reader client.Reader, parse ParseFunc[B], opts ...CacheOption) *Cache[B]

func (c *Cache[B]) Resolve(
    ctx context.Context,
    namespace string,
    ref *types.CABundleConfigMapRef,
) (*Resolved[B], error)
```

### Key and cached value

- Inject `mgr.GetAPIReader()` for refresh reads.
- Key entries by `(namespace, ConfigMap name, selected data key)`.
- Reader and parser are fixed for a cache instance.
- Store a parsed result or an error, plus expiry and LRU bookkeeping.
- Copy reference values rather than retaining caller-owned pointers.
- Return a fresh metadata wrapper on each successful resolution. Share only the
  parsed bundle, which consumers must treat as immutable. Mutating a returned
  reference or UID must not change the stored entry.
- A nil reference returns nil immediately: system trust needs no ConfigMap read
  or cache entry.

### Refresh policy

Proposed private defaults, not new operator configuration:

| Setting | Default |
| --- | --- |
| Successful resolution TTL | 1 minute |
| Failed resolution retry interval | 5 seconds |
| Retained entries | 256, bounded LRU including failures |

Successful TTL starts at read completion. Hits do not extend it. On a miss or
expiry, run the existing resolution/validation/parser sequence.

If refresh fails, replace the expired success with the failure and return the
error. Cache that error for five seconds to avoid repeatedly reading the same
broken ConfigMap across reconciliations. After that interval, retry on the next
acquisition. Do not negative-cache caller cancellation or deadline failures;
an expired success remains ineligible for reuse in those cases too.

Retain existing parser limits, including the 256 KiB input and 64-certificate
limits. Capacity bounds retained storage, not temporary parsing allocations.
`WithClock` replaces the clock for deterministic tests; it is not operator
configuration.

### Trust identity

A successful explicit-reference identity includes:

- Namespace, ConfigMap name and data key.
- ConfigMap UID.
- Canonical certificate digest from the existing parser.

PEM whitespace, certificate order, duplicates and irrelevant metadata do not
require client replacement if the validated identity is unchanged. A recreated
ConfigMap changes UID and therefore replaces the client, even with the same CA.

## 4. Shared client cache in common

`common/pkg/providerca/client_cache.go` contains the typed client cache. Both
provider adapters use this implementation, parameterized by their client type
and the parsed bundle type.

The client cache needs a canonical digest of each parsed bundle to detect trust
changes. The bundle type therefore satisfies a small constraint: it is
comparable (so a nil bundle can be rejected) and has `Digest() string`.
`*commonclient.ProviderCABundle` already provides the canonical digest computed
once during parsing, so no wrapper type or separate digest function is needed.

### Interface

```go
type Bundle interface {
    comparable
    Digest() string
}

type CAResolver[B any] interface {
    Resolve(context.Context, string, *types.CABundleConfigMapRef) (*Resolved[B], error)
}

type ClientRequest struct {
    Owner ktypes.NamespacedName
    UID   ktypes.UID
    Parts []string                       // transient connection and resolved authentication inputs
    Ref   *types.CABundleConfigMapRef    // trusted by all endpoints; nil means system
}

type Built[T any] struct {
    Value     T
    CloseIdle func()
}

type Builder[T any, B Bundle] func(callCtx, lifetimeCtx context.Context, bundle B) (Built[T], error)

func NewClientCache[T any, B Bundle](ca CAResolver[B]) *ClientCache[T, B]

func (c *ClientCache[T, B]) Acquire(ctx context.Context, request ClientRequest, build Builder[T, B]) (T, error)

// Manager lifecycle: Start(ctx), NeedLeaderElection(), idempotent Close().
```

A failed CA resolution is returned unchanged; adapters classify it with
`AsResolveError`.

These sketches define responsibilities, not a requirement for additional
forwarding interfaces. Missing mandatory construction dependencies are tested
developer errors.

### Acquisition algorithm

1. Extract current connection inputs and resolve credentials in the provider
   adapter.
2. Resolve the requested CA reference through the injected CA cache.
3. If resolution fails, return an error. Do not return an old client.
4. Compute the effective connection fingerprint.
5. Return the existing client if its fingerprint matches.
6. Otherwise build and store a replacement, closing the previous client's idle
   connections.
7. If construction fails, return the error, not the previous client. Retry on a
   later reconciliation through normal controller backoff.

**Client hits must never bypass CA-cache expiry checks.** No separate client TTL
is necessary: the CA cache controls trust freshness and each acquisition checks
the supplied connection/authentication inputs.

### One trust input per connection

- Each connection has one CA reference, trusted for all its endpoints: Kong uses
  it for the admin API and the OAuth token endpoint, Keycloak for its admin and
  token endpoints.
- Resolve the reference in the owner's namespace.
- Kong builds one transport; the token client and the OAuth-wrapped API client
  are distinct clients over it, so the access token is only added to API requests.
- Provider adapters retain their existing user-facing messages, condition reasons
  and blocked/retryable classification.

### Client identity and bounds

Use a length-framed SHA-256 fingerprint containing owner UID, connection and
authentication inputs, and the CA identity. Explicit markers distinguish
system trust from an explicit reference. Do not use an ambiguous concatenation.

| Provider | Connection/authentication inputs |
| --- | --- |
| Kong | Admin URL, issuer URL, client ID, resolved client secret |
| Keycloak | Admin/token URLs, client ID, username, resolved password |

Resolve credentials on every acquisition, including hits, so same-reference
credential rotation affects the fingerprint. Cached Kubernetes reads may still
be eventually consistent; no authoritative confirmation protocol is added.

Keep one entry per namespaced owner, with a private fixed 1024-entry LRU per typed
cache. Owner UID prevents reuse across owner recreation.

Never log credentials, input parts or fingerprints, or use them in events or
metric labels. Entries retain the fingerprint and client, not another copy of
raw input parts. Builders/token sources necessarily retain authentication
material required to operate.

## 5. Concurrency and lifecycle

### Synchronization

Each cache uses a short-held map/LRU mutex and 32 private, fixed striped mutexes,
selected by CA key or owner. Hold a stripe across lookup, refresh/build and
publication. Same-key callers serialize and recheck after waiting.

API reads, parsing, authentication, transport creation and disposal occur outside
the global map mutex. Stripe collisions can serialize unrelated requests; this
is an accepted tradeoff for avoiding keyed-lock registries and in-flight state.

Ordinary stripe waits are not context-interruptible. Keep external operations
bounded by their existing timeouts and the call context. Check `ctx.Err()` before
waiting, immediately after acquiring the stripe, and before publication or a
successful return. A canceled waiter must not build or receive a cached client
after it obtains the stripe. This favors simpler synchronization over immediate
wait cancellation.

Do not confirm snapshots or retry publication. An acquisition may use its
starting configuration snapshot during a concurrent update. A later acquisition
converges to the new inputs. No linearizable freshness guarantee is promised.

### Reconcile-scoped borrowers

Replacement, eviction and discarded candidates close idle connections only.
Do not cancel per-entry token contexts or interrupt active requests. A reconcile
holding a replaced client can continue making requests and refreshing tokens.

Require a finite idle-connection timeout on provider API and token transports,
including Keycloak's cloned transport. Use **90 seconds** as the private default
where no finite timeout is already set. This ensures that a borrowed, retired
client which opens another connection after eviction does not leave that idle
connection alive indefinitely. Idle timeouts must not impose an active-request
deadline.

Builders clean up partially constructed transports on failure. There is no
Release method, retired-client registry or borrower reference count.

### Manager lifetime

The shared client cache owns a cancellable lifetime context independent of
reconciliation. Initial Keycloak authentication uses the call context; later
refresh/reauthentication uses the lifetime context.

Register the shared cache as the manager Runnable, not duplicated factory
Runnables. Manager shutdown cancels the lifetime context and closes idle
connections on retained clients. Cancellation also reaches evicted clients'
token sources; finite idle timeouts bound their forgotten idle transports.

Close is idempotent. Acquisitions after close fail. Builds finishing after close
must not publish and must dispose of their candidate's idle transports.

## 6. Zone distribution and status validation

### Zone remains responsible for distribution

Keep Admin's source/bundle watches, repair logic and ownership/provenance checks.
Admin prepares all CA material before publishing: it validates every requested
source and merges them into one Zone bundle (limits apply to the union), validates
the provider configurations against it, then writes it with a single create or
update and re-validates it. Any failure returns an error, writes no bundle and
changes no provider configuration in that reconciliation. There is no per-provider freeze state or
selective field merging. A retained bundle is distinct from returning an expired
in-memory client after failed refresh.

The bundle is the Zone's trust boundary: a source configured for one provider is
trusted by every provider of the Zone, and an accepted source change rotates all of
them.

Zone source validation can continue to call the shared uncached resolver as part
of copying/repairing ConfigMaps. It is not a provider-client acquisition path.

### Provider status uses the same CA cache

Inject the same CA cache into Gateway and Identity status-validation paths as
into their client cache. Do not maintain a second, authoritative per-reconcile
CA-resolution path in provider adapters or handlers.

- Gateway `Validate` performs endpoint/trust validation without resolving
  authentication credentials or creating a client.
- Identity candidate/committed-snapshot trust validation uses the injected CA
  cache and retains domain-specific checks and error/condition mapping.
- Status and acquisition share the same eventual-freshness policy. Neither gets
  an immediate-observation exception.
- Keep condition-before-status-update and ObservedGeneration behavior.

Gateway and Identity have no CA ConfigMap watches. The CA cache reads through
the uncached APIReader, so it needs no ConfigMap informer. Ordinary dependency
watches remain: Gateway changes enqueue Routes and Consumers, IdentityProvider
changes enqueue Realms and Realm changes enqueue Clients. A CA change is observed
on the next reconciliation of each resource after its cache entry expired, which
is either a dependency event or the periodic requeue (default 30 to 51 minutes).
There is no shorter timer, background refresh or expiry-triggered fanout, so a
bundle rotation has no wall-clock bound such as the one-minute cache TTL.

## 7. Concrete implementation and removal map

| Location | Planned change |
| --- | --- |
| `common/pkg/types/providertls.go` | Keep existing shared reference schema. |
| `common/pkg/providerca/{resolve,origin}.go` | Keep resolution/provenance primitives; add the bounded CA cache (`cache.go`) and the typed client cache with manager lifecycle (`client_cache.go`) alongside them. |
| `common-server/pkg/client/provider_tls.go` | Keep parser and TLS helpers; ensure provider transports have finite idle timeouts. No dependency on `common`. |
| `gateway/pkg/kongutil/trust_factory.go` | Keep thin `ClientFor`, trust-only `Validate`, error mapping and HTTP/OAuth builder. Remove duplicated entry/owner maps, LRU, epochs, `begin/end/stillCurrent/confirm/publish`, retries, race hooks and lifecycle management. Use supplied Gateway snapshot rather than confirmation rereads. |
| `identity/pkg/keycloak/factory.go` | Keep `ServiceFor`, `RealmOwner`, connection extraction and builder. Retain one Realm read per acquisition for owner-only inputs and persisted cleanup snapshot; check UID/completeness/HTTPS, resolve password, delegate. Remove its cache implementation, `Invalidate`, confirmation loops, hooks and lifecycle management. |
| `identity/pkg/keycloak/trust.go` and IdP/Realm handlers | Keep thin mapping helpers; route provider CA validation through the injected cache, not direct resolution. |
| Gateway and Identity `cmd/main.go` | Construct/inject CA and client caches, register shared Runnable, remove invalidator wiring. |
| Gateway trust controller and Gateway/Route/Consumer setup | Remove invalidator fields/calls, CA ConfigMap watches, the Gateway CA-reference index and the ConfigMap predicate. Keep the Gateway-to-Route/Consumer mapping and its indices. |
| `identity/internal/controller` Realm/Client/IdentityProvider setup | Remove CA ConfigMap watches, `trust_fanout.go` and the CA-reference indices. Keep IdentityProvider-to-Realm and Realm-to-Client mapping. |
| Admin Zone trust handler/watch setup | Retain distribution, the durable Zone bundle and source/bundle watches. Prepare all CA material before publishing; any failure stops publication. |
| Factory/controller tests and exported test hooks | Delete obsolete epoch/immediate-invalidation expectations and hooks. Replace with shared-cache contract tests and thin adapter tests. |

Do not undo unrelated working-tree changes. The prior endpoint-helper extraction
does not need another rewrite for this design.

## 8. Dependencies and compatibility

### Blockers

No technical blocker required provider-local CA/cache implementations. The
required refactoring was:

- Remove direct CA resolution from Identity status/candidate/snapshot validation
  paths and inject the same cache. Make Kong validation trust-only rather than
  reuse its full connection/credential resolver.
- Centralize transport retirement and finite idle-timeout defaults while
  preserving provider-specific authentication, TLS and request-timeout policies.
- Preserve Realm snapshot/cleanup domain safeguards and its necessary owner
  lookup, but remove repeated CA reads and acquisition confirmation machinery.

Eventual trust observation was accepted by the user and applies equally to
status validation.

No module dependency changes are needed: `common` stays independent of
`common-server`, and `common-server` does not import `common`. Gateway and
Identity already depend on both and wire them together.

The unpublished Gateway `spec.admin.issuerTLS` field was later removed with the Zone bundle; no other CRD, JSON or reference compatibility change is made. Internal constructor
and injection interfaces change. Freshness intentionally changes: trust is no
longer authoritatively reread and confirmed on every acquisition or provider
status check.

## 9. Test plan

Use Ginkgo/Gomega and deterministic clock seams. Use envtest for Kubernetes API
behavior, not new Kubernetes mocks. Keep provider integration tests where they
exercise actual authentication and transport behavior.

### Shared CA-cache tests

- Nil references perform no reads.
- Namespace, ConfigMap name and key isolate entries.
- Cache hits, fixed expiry and refresh after rotation or recreation.
- Missing/forbidden/read failures, missing key/label, provenance and invalid PEM.
- No expired-success fallback after refresh failure.
- Negative-cache retry interval and cancellation behavior.
- Bounded retention and refresh after eviction.
- Returned metadata mutation cannot alter cached identity.

### Shared client-cache tests (in `common`, with a test bundle type)

- Builder counts demonstrate reuse and replacement.
- A reference and nil/system trust stay distinct; nil is not resolved.
- Equivalent canonical CA digest reuses a client.
- Endpoint, credential, owner UID and CA identity changes replace clients.
- CA failure prevents both builder invocation and old-client return.
- Build failure returns an error; later acquisition can recover.
- Eviction/replacement/shutdown dispose idle connections correctly.
- A borrowed client remains usable after replacement, including a new request
  after retirement; its subsequently idle connection expires.
- Reconcile cancellation does not cancel lifetime token refresh.
- Same-key concurrent calls serialize without duplicate publication.
- Different stripes can progress during a blocked build.
- Canceled stripe waiters cannot succeed after acquiring the lock.
- Shutdown during construction prevents publication and cleans the candidate.

### Provider/controller tests

- Preserve error messages, condition reasons and blocked/retryable mapping.
- Kong reaches API and token servers of distinct CAs through one combined bundle
  and one transport without sending the access token to the token endpoint.
- Preserve Keycloak initial authentication, refresh and credential rotation.
- Status validation and client acquisition use the same cached trust policy.
- Retain Zone distribution/repair, status and deletion-recovery envtests.
- Remove tests for the superseded immediate-invalidation/epoch protocol rather
  than retaining that protocol just to satisfy them.
- Use explicit timeout and interval for Eventually assertions.

## 10. Implementation sequence and verification

1. Add the common CA cache and its focused tests around the existing resolver.
2. Add the shared client cache in common and its lifecycle tests, plus the
   finite idle-timeout default in the common-server transport helper.
3. Migrate Gateway and Identity builders and status validation to the caches.
4. Delete superseded per-provider caching/invalidation code and obsolete tests.
5. Update documentation and run affected module verification.

Use Makefile targets, not direct Go test/build/lint commands:

- `make test` in common, common-server, gateway, identity and admin.
- Root `make verify MODULES="common common-server gateway identity admin"`.
- Wider builds through existing Make targets, restricting to active modules if
  nested worktrees would otherwise be included.

Update the environment/Zone documentation and Gateway/Identity architecture
documents with TTL observation, fail-closed refresh and reconcile-scoped use.
Conflicting immediate-invalidation statements in
`docs/design/provider-ca-references.md` were revised with the implementation.

## 11. Tradeoffs and review status

- A fresh cached CA can mask deletion or invalid changes until TTL expiry.
- Unused clients have no wall-clock refresh promise; observation requires a later
  reconcile/acquisition.
- Actual recovery latency includes controller scheduling/backoff, not just the
  five-second failed-resolution interval.
- Fixed capacities and stripes are private defaults, not workload-derived
  guarantees or a new operator-tuning surface.
- Stripe waits favor simple synchronization over immediate cancellation.
- Finite idle timeouts allow retired borrowed clients to finish without requiring
  borrower tracking.

Two initial design reviews identified the need to unify provider status reads
with the CA cache and bound retired-transport idle lifetime. This document
incorporates those changes, metadata-copy isolation and explicit canceled-waiter
behavior. The user later moved the client cache from `common-server` to `common`
to avoid a new module dependency. **The implementation has not been reviewed
yet.**
