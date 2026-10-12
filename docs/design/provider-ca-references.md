<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
SPDX-License-Identifier: CC0-1.0
-->

# Provider-local CA references

Status: accepted; implemented (stages 1–4); integrated Make verification and implementation review recorded below. This supersedes the earlier provider CA proposal (`docs/design/provider-ca-bundles.md`, not present in this repository) and the subsequently discussed global aggregate alternative. Client caching and trust freshness were later replaced by the shared caches of `docs/design/reconcile-client-cache.md`, and per-source copies were later replaced by one combined CA bundle per Zone; the sections below describe that behavior.

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

Implementation map:

- Contracts: `common/pkg/types/providertls.go` (refs, label); `common/pkg/providerca` (copy origin tuple, hash, name, annotation keys, the runtime metadata validation and the uncached reference resolver `Resolve` with typed failure kinds, used by Admin, Gateway and Identity; the PEM parser is injected to keep `common` independent of `common-server`); strict PEM/merge/transport helper `common-server/pkg/client/provider_tls.go`; label-filtered ConfigMap caches `common/pkg/client/providerca_cache.go`; the shared bounded CA cache and typed provider client cache `common/pkg/providerca/{cache,client_cache}.go`, used by Gateway and Identity for status validation and client acquisition; URL primitives `common/pkg/util/urlutil` (HTTPS check, canonical host) behind each component's own URL policy; Keycloak endpoint derivation `identity/api/endpoint` shared by Identity snapshots and Admin's committed-Realm gate.
- Admin: `admin/internal/handler/zone/trust.go` (sources, Zone bundle, `preparedIdentityProvider` staging, committed-Realm gate incl. admin-token target and issuer-hostname-only moves, effective issuer for dependents), `gateway.go`, `identity.go`, `routes.go`, `links.go`; source/copy watches `admin/internal/controller/zone_trust_watch.go`. Generated children are staged from uncached reads.
- Gateway: `gateway/pkg/kongutil/trust_factory.go`, `gateway/internal/controller/trust.go`, `TrustReady`.
- Identity: shared Realm snapshot classifier `identity/api/v1/realm_snapshot.go` (`ClassifyRealmSnapshot`, used by Identity and by Admin's committed-Realm gate), `identity/pkg/keycloak/{factory,trust,target}.go`, Realm snapshot commit `identity/internal/handler/realm/snapshot.go`, Client deletion through persisted snapshots. Gateway and Identity have no CA ConfigMap watches (see `reconcile-client-cache.md`).

Consumer provenance rule: every reference requires the label. A referenced ConfigMap whose name matches `provider-ca-<40 hex>` or that carries any origin annotation is treated as an Admin-managed copy and requires complete, hash-consistent provenance, matching Zone and present Environment labels, no ownerReferences, a reference name equal to the copy name and key `ca.crt`; other labeled ConfigMaps referenced by manually authored providers remain ordinary local sources.

Test coverage (Ginkgo/envtest) includes: strict PEM and real TLS servers; copy creation/no-op/rotation/collision/repair; unchanged providers on source, copy or validation failure and publication after repair; no copy written while another source is invalid; latched Zone with committed Realm A rejecting IDP B before any IDP, Gateway issuer, managed-route issuer or discovery change (nil and custom trust); issuer-hostname-only and admin-token gating; a rejected IDP target move leaving both the Zone bundle and the published provider unchanged, also when its source was overwritten or a distinct source was staged, and the rotation taking effect once the configuration is accepted; deterministic, deduplicated union of all Zone sources published to every provider including those without a source; union limits; cross-Zone isolation; system trust with a retained bundle after all sources are removed; retained copies after Zone deletion with manual rotation; consumer provenance (valid copies, partial/spoofed metadata, rejection after cache expiry); Realm snapshot commit/conflict/legacy cases and TrustReady when the IDP is not ready; shared CA/client cache contracts (TTL, failure caching, identity, concurrency, lifecycle, retired idle connections); restricted-identity RBAC against each module's packaged `role.yaml` + `leader_election_role.yaml`; server-side selector across watch expiry/relist. This list is not a claim that every sentence of this design has a dedicated test. Known limitations: a crash between a pre-feature remote Realm creation and its status write can leave an empty snapshot that is treated as having no remote obligation; real Keycloak/Kong servers are not exercised.

Additional acceptance coverage includes actual Gateway expired-token refresh over the shared Gateway transport after cancellation of the construction context, rather than only reuse of an unexpired token. Historical Realm cleanup tests use the production factory and controller with real TLS test servers and a rotated retained Admin copy, without a live Zone, source ConfigMap or IdentityProvider: the rotation is used once the cached trust expires, TLS failure retains finalizers, and successful cleanup reaches the historical token/API endpoints after server certificate rotation. These are test-server checks, not deployments against real Kong or Keycloak.

## Decision and goals

Admin accepts Zone-local CA ConfigMap references and combines all of them into one durable CA bundle per Zone beside the generated providers. Every Gateway and IdentityProvider of the Zone references that bundle, including providers whose own Zone entry names no source. Gateway and Identity resolve their local reference and replace clients when trust changes. Admin discovery and endpoint validation use the same Zone bundle. No global aggregate, mounted-file reload, Pod restart or process-wide trust mutation.

**Zone trust boundary:** custom anchors are additive to system roots and scoped to the Zone: an anchor configured for one provider is trusted by every Gateway and IdentityProvider of that Zone, but never by another Zone. A source change rotates trust for all providers of the Zone. This trades per-provider isolation for one bundle, one reference and one write per Zone.

User requirement: deleting a Zone must not withdraw trust needed by children for remote cleanup. Existing Zone deletion retains children, not cascading deletion. Durable copies survive Zone/source deletion, but retaining certificates alone cannot preserve deleted credentials, providers or OAuth identities. The generated namespace and copies must remain until cleanup finishes.

Accepted and implemented policies: combine Zone-local sources into one Zone bundle; publish no provider change while any requested source or copy fails; retain copies until manual cleanup; stop synchronization on Zone deletion; rotate via live sources or directly in retained copies after deletion. Block target migration of committed Realm cleanup snapshots rather than add a cleanup ledger. No automatic GC, post-Zone synchronization, live detachment state, lease protocol, new recovery sweep, mTLS or serving-certificate management.

## Pre-feature baseline (historical)

The following observations motivated the design; they describe the implementation before this feature, not its current behavior. The implementation map above describes the delivered changes. Later sections retain the original requirement wording as the accepted implementation contract.

- `admin/internal/handler/zone/handlingContext.go` creates lowercase `<environment>--<zone>`, distinct from the Zone's namespace. `handler.go` has provisioning/readiness barriers; Delete intentionally retains all children.
- `zone/identity.go` and `zone/gateway.go` replace generated specs. `zone/oidc.go` validates discovery metadata, but default HTTP clients lack custom trust.
- `common/pkg/client/client.go` scoped Get rejects objects lacking Environment labels; ordinary source ConfigMaps need an explicitly injected raw reader, not weakened global scoped-client behavior.
- `gateway/pkg/kongutil/client_factory.go` caches connection configurations without trust identity and shares API/OAuth transport. `identity/pkg/keycloak/factory.go` retains cached services, authenticates under its cache lock and has separately instantiated controller factories.
- `identity/internal/handler/realm/handler.go` deletes through historical Realm status; Client deletion currently treats a not-ready Realm as permission to skip remote cleanup.
- `common-server/pkg/client/cert-refresher.go` appends into an in-use pool; it cannot safely implement replacement/removal. New code must not change the internal Secret Manager trust configuration.

## API contract

Add optional `tls` to each Zone gateway and IDP configuration:

```yaml
tls:
  caBundleConfigMapRef:
    name: private-provider-ca
    key: ca.crt
```

Example source (PEM abbreviated):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: production # Zone namespace
  name: private-provider-ca
  labels:
    cp.ei.telekom.de/provider-ca: "true"
data:
  ca.crt: |
    -----BEGIN CERTIFICATE-----
    ...public CA certificate...
    -----END CERTIFICATE-----
```

Use a shared value type `CABundleConfigMapRef{Name, Key}` and optional `ProviderTLS{CABundleConfigMapRef ...}` under `common/pkg/types`. Name/key are required for a present reference. Omitted tls/ref means system trust; no key default, namespace field, optional fallback, inline PEM or insecure flag. This is equivalent in locality to `ConfigMapKeySelector`, deliberately omitting its optional field. Read `data[key]`, not binaryData. Validate Kubernetes name/key syntax in schema/runtime.

**New setup requirement:** every referenced source CA ConfigMap must carry `cp.ei.telekom.de/provider-ca: "true"` before configuring its reference. Admin durable copies require the same label. Require the exact string value at runtime, including on uncached reads; missing or mismatched labels make the source invalid even when its PEM is valid. This label opts into provider-CA caches and Admin's source watch; it does not claim Admin ownership of a source, confer authorization or replace copy provenance checks. Shared sources need no Zone ownership labels. Preserve the label on rotation and same-name recreation, and label a newly selected source before changing refs.

| Resource | Field | Lookup namespace / use |
| --- | --- | --- |
| Zone gateway | `tls.caBundleConfigMapRef` | Zone namespace / source of the Zone bundle |
| Zone IDP | `tls.caBundleConfigMapRef` | Zone namespace / source of the Zone bundle |
| Gateway | `spec.admin.tls.caBundleConfigMapRef` | Gateway namespace / Kong API and OAuth token |
| IdentityProvider | `spec.tls.caBundleConfigMapRef` | IDP namespace / admin and tokens |
| Realm | `status.tls.caBundleConfigMapRef` | Realm namespace / persisted cleanup connection |

Admin publishes the same Zone bundle reference to every provider, or nil (system trust) when the Zone names no source. A Gateway trusts its one reference for both the Kong API and the OAuth token endpoint, through one transport; both URLs must be https when it is set. The Zone entries remain per provider so administrators can declare which provider needs which source, but they only contribute to the union. Reject cross-namespace IDP-to-Realm inheritance of a nonnil local trust reference; existing nil-ref configurations remain compatible. Generated resources are colocated. Multiple provider entries can reuse a source/key; the union is deduplicated.

## Zone CA bundle identity and lifecycle

Admin reads every distinct source of the Zone with the strict parser, merges their certificates into one canonical, deduplicated PEM (sorted by DER) and writes it to the generated namespace before publishing any referencing child configuration. The size (256 KiB) and certificate (64) limits apply to each source and to the union; an oversized union blocks like an invalid source. The bundle is written with a single create or update, so no partial multi-copy state exists.

Bundle identity is `(Zone UID, Zone namespace/name)`, canonically length-prefixed and SHA-256 hashed. Name: `provider-ca-<first 40 hex characters>`; fixed output key `ca.crt`. The name is stable for the Zone incarnation and independent of the sources; a recreated Zone has a new UID and bundle name. Full origin hash and tuple annotations reject collisions, not just the truncated name.

The bundle has the existing Environment/Zone/domain labels, `cp.ei.telekom.de/provider-ca: "true"`, Zone UID provenance and annotations for the Zone origin tuple and its full hash. It carries no source ConfigMap or key annotation, because it does not stem from a single source; the source membership is the Zone spec, and Admin's source watch maps sources through the Zone index. Consumers validate the Zone provenance only; neither sources nor the Zone need to exist for cleanup. Copies of a single source, written by earlier versions of this feature, carry both source annotations and remain valid for persisted references; Admin no longer writes or deletes them. No source UID/resourceVersion, accepted-data digest or management-state annotations. Bundles have no ownerReferences/finalizers and are excluded from generic subresource readiness/cleanup. Only changed PEM or essential ownership metadata triggers a write.

Before mutation, verify complete origin and labels; unexpected ownerrefs, immutable objects or collisions block without overwrite/adoption. Reject modifying a retained generated child carrying another Zone UID. Pre-feature children may receive initial provenance only when existing Environment/Zone labels match. Reusing old names after Zone recreation requires deliberate migration/teardown; no silent adoption.

No automatic bundle deletion, including on source/ref removal, Zone deletion or last-consumer deletion. Removing all sources publishes nil references but retains the bundle for persisted cleanup references. A valid source change replaces the complete union, removing anchors no source contains anymore; retain overlap during server rollout. Deliberate rotation/withdrawal differs from accidental source failure.

**Zone-wide rotation scope:** an accepted change of the Zone's sources changes the certificates of every reference to the Zone bundle, including historical Realm cleanup snapshots. Admin validates the provider configurations (IDP target against committed Realm snapshots, HTTPS endpoints) against the new union before writing it, so a rejected configuration leaves both the bundle and the published providers unchanged. Accepted removals withdraw anchors from all consumers of the Zone; a removed CA is not retained for history.

## Admin reconciliation

### Coherent target and trust candidates

Normalize URLs with net/url: absolute hostname, no userinfo/fragments; lowercase scheme/DNS hostname, normalize default ports and empty path to `/`. Preserve escaped paths, trailing slashes and queries allowed by existing endpoint contracts; never collapse different base/realm paths into one origin. Custom-trust targets require HTTPS; nil-ref URL behavior remains compatible. Derive URLs with existing helpers before comparing.

Gateway target identity is `(admin API URL, OAuth issuer URL)`. IDP identity includes admin URL, derived admin-token URL and issuer/discovery URLs used by Admin. Realm target includes Realm name and its issuer/admin/token URLs. Source selection is the canonical origin/local reference, not source UID or metadata version. Persist effective discovery issuer in an Admin-owned IDP annotation when publishing complete connection candidates because current IDP spec lacks that Zone field. On upgrade without annotation, use only unambiguous existing values; otherwise require fresh source validation before setting the baseline.

Keep the two issuer derivations distinct in the candidate: Realm issuer derives from IDP AdminUrl via existing Keycloak helpers; Gateway OAuth/Admin discovery issuer derives from Zone IssuerHostname with its existing fallback. An issuer-hostname-only change must not disappear from comparison. Preserve explicit token-link precedence (preset TokenUrl, then IDP TokenUrl); those published links are not replacements for internal admin-token targets.

1. Ensure generated namespace independently of provisioning barriers. Direct-read current children and stage complete provider candidates: target URLs, credential inputs, source selections and effective trust.
2. Resolve sources with uncached `mgr.GetAPIReader()`, collect failures, require the source label, strictly validate and persist valid copies independently of provisioning. The APIReader can retrieve unlabeled objects, but they fail runtime validation. Direct-read/validate actual copy data, required label and complete origin before use; invalid copies block acquisition, including client-cache hits.
3. Unchanged selection AND target may retain a valid copy on source failure. A changed custom-trust target requires fresh valid selected source, even if the ref is unchanged. This validates declared trust, not remote acceptance; normal verified TLS establishes the latter.
   Before publishing a changed IDP target, inspect committed generated Realm snapshots using current namespace/ownership indexes. If any target is incompatible, return a blocked migration-required error before the IDP, dependent Gateway issuers or discovery change. Do not rely on existing latched Zone readiness barriers. Identity independently enforces the same rule. Apply this narrow cleanup-safety restriction to nil-ref/system-trust configurations too and document the behavior change. If credential/realm dependencies cannot be established safely, block rather than reuse rover credentials against a new issuer. No migration ledger is introduced.
4. Prepare first, in this order with early returns: read, validate and merge every requested source (no write); stage the IdentityProvider candidate, including the committed-Realm target check; check Gateway endpoints; only then write and re-validate the Zone bundle and publish providers. Any failure returns an error, writes no bundle and changes no provider in that reconciliation. There is no frozen-candidate state and no selective merging of desired and published fields.
5. Every spec replacement consumes the prepared candidate rather than raw Zone fields. An IdentityProvider failure stops the pipeline before Gateways, routes and presets. Gateways are published in order; a failing Gateway is skipped and its error returned.
6. Discovery runs on each eligible reconciliation for a coherent IDP candidate using the validated Zone bundle, unless an explicit TokenUrl supplies the link (preset first, then IDP). Results are memoized by issuer only within that reconciliation; persisted discovery metadata is not reused across reconciliations. There is no discovery-trust annotation; the effective discovery-issuer annotation described above remains a separate target-consistency input. A reconciliation that fails to prepare the IDP issues no discovery or metadata publication. Preserve issuer/HTTPS/size/10-second checks and deny redirects.
7. Removing some refs rotates the bundle to the remaining union. Removing all refs intentionally chooses system trust for every provider through normal provisioning; the bundle remains retained.

Zone `TrustSourcesReady` reflects synchronization of requested sources, not remote connectivity. Source failure sets False with reason `SourceInvalid`, copy failure with `CopyInvalid`; existing latched Ready must not hide it. Copy snapshot errors cannot be hidden by source success. Consumers set `TrustReady` for resolved trust. Conditions carry ObservedGeneration and are set before status updates. Use `BlockedErrorf` for stable configuration faults and `RetryableErrorf` for transient reads/writes; preserve partial-success requeue.

## Failure policy

| Event | Admin | Consumer |
| --- | --- | --- |
| First source missing/invalid/forbidden, including absent or mismatched provider-ca label | No custom ref; block provider creation | Never silently fall back for a configured reference |
| Any requested source or the bundle invalid, or the union over limits | Return an error; write no bundle for source/union failures; publish no provider change | Continue previous published configuration |
| Source recreated/updated with valid PEM | Replace the union; bundle name remains unchanged | Replace on the next reconciliation after cache expiry |
| Removal of the last tls/ref | Publish nil through normal provisioning; retain the bundle | Use fresh system-root clients on the next reconciliation |
| Copy deleted, live matching-UID Zone/source valid | Repair/recreate after provenance checks | Block while absent |
| Copy missing/malformed/forbidden or required label/provenance invalid | Repair from valid source only for live non-deleting matching-UID Zone and subject to pre-mutation origin/label checks; never adopt a collision | Reject new acquisitions, including cache hits |
| Zone deleting/gone | No automatic synchronization/repair | Resolve retained copy; errors block cleanup |

Consumer caches are not a durable fallback. Missing copy plus missing source needs external recovery. A valid source may repair a live Zone's copy; consumers do not trust cached clients after current resolution fails. Source deletion is not revocation: published providers keep using their existing copy. Withdraw anchors by valid source replacement while live or by directly editing retained copies after Zone deletion.

## Zone deletion and retained operation

At deletionTimestamp, stop source refresh and child-ref mutation; retain namespace/copies/children, then execute existing Zone Delete and remove the existing finalizer. Source availability never blocks Zone deletion. Writes require a direct check of a live, non-deleting matching-UID Zone; an already in-flight write can race deletion because Kubernetes has no cross-object transactional freeze. No write is started after deletion is observed.

After Zone deletion completes, copies are authoritative retained configuration and original sources may disappear. No post-Zone source rotation is promised. Live emergency rotation replaces selected source PEM; direct live-copy edits may be overwritten. After deletion/finalizer completion, back up and directly replace retained `data.ca.crt` with validated overlap PEM, preserving the required provider-ca label and origin metadata. Consumer watches activate the change. No detachment/reattachment state is introduced.

Teardown order: remove remote business children while retaining provider connection configuration, Realm snapshots, rover OAuth Client/Realm and credentials; wait for finalizers/verified remote absence; then remove providers, copies and generated namespace. Include active and terminating references in the audit. Zone deletion initiates none of this cascade. Deliberate deletion of copies/namespace is outside the CA-retention guarantee; backups and privileged teardown procedure are required.

### Identity cleanup connection snapshots

Append trust ref to historic Realm connection status. A complete existing snapshot is a committed cleanup obligation regardless of Ready or previous request success. Its Realm name/normalized issuer/admin/token target cannot migrate from A to B. Reject differing live targets, preserve A, and make no B request. Partial nonempty legacy snapshots are ambiguous: block and require operator repair.

Matching-target credential inputs and validated refs may refresh. For a new Realm with no connection status, persist the complete B snapshot before the first request; later failure can clean B without losing an earlier A obligation. Upgrade complete legacy status lacking TLS only when live target matches. Invalid pending trust preserves the prior complete snapshot. Terminating operations use historical status and resolve its local trust without requiring a live IDP.

Sequence: stage/local-validate target and trust; set conditions; perform a targeted optimistic status patch protecting owner UID/resourceVersion and preserving unrelated fields; only then resolve credentials, authenticate and perform remote calls. Conflict/status failure means no remote request; bounded retry rebuilds from current state. Same-target trust remains committed if subsequent authentication/provisioning fails.

Successful early commit must update the handler object's status/resourceVersion from the patch response before generic controller status persistence runs. On commit failure, discard staged connection changes from the handler object so generic error handling cannot accidentally persist them. Keep this a targeted status-writer integration, not a generic-controller rewrite.

Preserve existing credential compatibility: configured Secret Manager references remain references; existing literal password inputs remain their original literals. Never replace a persisted reference with its resolved password or persist tokens/PEM. Resolve into transient runtime copies.

Rejecting target migration of committed Realms is an intentional behavior change: create a distinct replacement Realm/provider, migrate dependents, clean old resources using historic configuration, then retire them. Finish cleanup before reusing the same remote name via CR recreation. Clearing status is not a migration procedure.

Split Realm lookup from readiness checking for Client deletion. An existing Realm's persisted connection can be used while not Ready; trust/auth/read/validation failures return errors and keep finalizers. Remove the broad not-ready/BlockedError-to-orphan branch; preserve explicitly missing-Realm orphan behavior as an existing limitation. Confirmed remote absence remains idempotent success. Gateway cleanup still requires the referenced Gateway; this feature does not snapshot or enforce the whole deletion graph.

## TLS helper and client lifecycle

New pure helper in `common-server/pkg/client/provider_tls.go`: strict full-input PEM parsing, canonical DER deduplication, fresh root pools and transports; no file watcher or mutation of default/internal clients. Maximum selected input 256 KiB and 64 certificates; bounded reads/normalization. Accept headerless CERTIFICATE blocks separated by whitespace; reject empty, malformed DER, keys, other blocks and garbage. CA anchors require valid CA basic constraints and CertSign when KeyUsage is present. Direct self-signed non-CA anchors require a valid self-signature and a constrained exact-leaf acceptance profile tested on supported Go versions; reject arbitrary non-self-signed leaves. Define allowed basic constraints so a trusted non-CA anchor cannot authenticate a different certificate it signed; apply post-verification exact-leaf restriction if normal verifier is insufficient.

Do not reject staged future-dated certificates merely at load time; requests still enforce hostname, time, chain and server-auth verification. Custom trust adds to a fresh system-root pool; failure to obtain system roots is an error, not custom-only fallback. Nil refs preserve system behavior. Never set InsecureSkipVerify or weaken ServerName verification. Preserve current per-operator timeouts/TLS minima/metrics. Reject redirects on credential-bearing token/API clients; Gateway API and token clients share one transport trusting the Zone bundle; only API requests carry the access token.

The CA cache and client cache in `common/pkg/providerca` are constructed once per manager in main.go and shared by its controllers (see `docs/design/reconcile-client-cache.md`). Gateway acquires clients for the supplied Gateway; Identity reads the owner Realm's persisted connection status once per acquisition and checks UID, completeness and HTTPS. Realm normal reconciliation commits status before acquisition; no unpersisted candidate bypasses the snapshot.

CA resolutions are cached for one minute after a successful read and five seconds after a failure, without stale fallback. Status validation and acquisition use the same cache. Client identity is a length-framed digest of owner UID, connection inputs, transiently resolved credentials and the CA identity (reference, ConfigMap UID and canonical PEM digest). Metadata-only changes do not rebuild. There are no epochs, confirmation re-reads or publication retries; one client per owner, bounded by a 1024-owner LRU, idle-closed on replacement and eviction.

Trust changes are observed eventually, after cache expiry and a later reconciliation. Already-acquired clients may finish their reconciliation with old trust. There is no instantaneous revocation or global linearizability.

Initial Identity authentication uses reconcile context. Cached OAuth refresh/reauth uses shared cancelable manager context carrying that generation's token client, not an expired construction context or per-generation cancellation goroutine. API requests use reconcile contexts; preserve existing finite per-request timeout contracts including OAuth-created clients. The Gateway token HTTPClient is distinct from the API client but uses the same transport as its OAuth Base; no global lock spans network auth. Replacement evicts old cache entry and closes idle connections; existing callers may finish, and provider transports close later idle connections after at most 90 seconds. No lease/release protocol, timed drain cancellation or forced old-handle revocation. Handlers retain no handles beyond reconciliation; obsolete generations are collectible after callers finish. Shutdown cancels shared refresh context and closes current owned transports. Test bounded cache/history, caller lifetime, LRU and shutdown.

## Watches and recovery

- All three managers use the server-side ConfigMap label selector `cp.ei.telekom.de/provider-ca=true` for informer list/watch requests. Only matching sources/copies enter their ConfigMap caches. Authoritative source/copy resolution still uses the uncached APIReader and checks the required label; cache absence alone is not an authoritative existence check.
- Admin maps source events through namespace-qualified source-ref indexes, and copy events through verified origin tuple/hash and Zone UID metadata. The shared label alone never identifies an Admin-owned copy. Source/copy create/update/delete map to live Zones; namespace/ref mapping must also work for a source shared by multiple Zones. Ignore irrelevant data changes semantically, not generation-only events; required-label/provenance changes are relevant. Zone spec change is independently watched; no reconcile loop from no-op copy writes.
- In Admin, a matching-to-nonmatching label change exits the server-side filtered watch as a delete event. Enqueue affected Zones even if the removal object no longer matches the selector; do not add a label-checking predicate or mapper guard that rejects it. Use namespace/ref indexes for sources and verified origin metadata for copies, independently of the removed label. A nonmatching-to-matching change enters as an add event and must recover an initially unlabeled source or copy once runtime validation succeeds. Changes to sources that remain unlabeled are not watched.
- Gateway and Identity have no CA ConfigMap watches or CA-reference indexes. Gateway changes enqueue active/terminating Consumers/Routes; IdentityProvider changes enqueue Realms; Realm changes enqueue Clients. A CA change is observed on the next reconciliation after the CA cache entry expired: a dependency event or the jittered periodic/error requeue (blocked fallback approximately 30–51 minutes). There is no shorter timer, background refresh or recovery SLA. Do not claim immediate revocation.
- Register indexes before managers start; use fully qualified refs/environment checks. Startup reconciles existing resources. Log mapping/list failures.

## Permissions and trust boundary

Implemented default for dynamic generated namespaces: all three managers receive clusterwide ConfigMap get/list/watch, with server-side label-filtered ConfigMap informers. In `admin/cmd/main.go`, `gateway/cmd/main.go` and `identity/cmd/main.go`, the shared cache configuration sets `cache.Options.ByObject` for `&corev1.ConfigMap{}` with `cache.ByObject.Label` matching `cp.ei.telekom.de/provider-ca=true`. This is a per-GVK selector, not a global `DefaultLabelSelector`. Admin's existing domain=admin selectors for Identity Clients and Gateway Consumers/Routes are preserved.

The selector reduces cache memory and exposure of unrelated ConfigMap data through informer list/watch traffic; it does not narrow RBAC. Each operator can still directly get/list/watch any ConfigMap with its credentials, and `mgr.GetAPIReader()` remains uncached and outside cache selectors. Refs, labels and event predicates are not authorization boundaries. No Secret grants are added. Operators are privileged platform components, not hostile-tenant sandboxes. Stronger isolation requires scoped deployments rather than implicit guarantees from labels.

Dependency audit: current production Go paths in Admin, Gateway, Identity and shared `common` controller/client code contain no ConfigMap reads/watches that directly block this filter; the Gateway/common ConfigMap usages found are test fixtures. `common-server`'s `MergeConfigMap` is Viper configuration merging, not a Kubernetes read, and the Secret Manager API client uses HTTP independently of these manager caches. Implementation gate: re-audit controller/webhook/helper and dependency paths, including leader election, against the exact binaries before enabling filtering. Any other ConfigMap reader must use a direct reader, or its watch must use a separately, narrowly configured cache; do not disable the provider-CA filter wholesale. Verify those paths continue working with unrelated ConfigMaps absent from the manager cache.

Admin additionally gets ConfigMap create/update/patch clusterwide for copies, not delete. Code enforces derived namespace and origin checks, but RBAC does not express these predicates. Source writers change trust of referencing providers; copy/provider-spec/status writers directly control those providers. Restrict these writes to authorized platform administrators. Copies are public trust material, not confidential stores.

Gateway/Identity have no new ConfigMap writes. Their namespace leader-election roles no longer grant ConfigMap mutations; Lease/event permissions remain. Keep operator namespaces distinct from provider namespaces and audit effective rendered RBAC. Do not add provider roots to Secret Manager's bundle, default transports, or process environment.

## Implementation stages and verification

1. Contracts/shared helper: common types, `admin/api/v1/zone_types.go`, `gateway/api/v1/gateway_types.go`, `identity/api/v1/{identityprovider_types,realm_types}.go`, schema/runtime validation and real TLS tests.
2. Downstream resolution: manager readers/factories, shared CA/client caches and OAuth contexts, Realm snapshot/Client deletion handling. No CA ConfigMap watches.
3. Admin copies: new zone/trust.go, independent namespace setup, pre-barrier publication, committed-Realm target eligibility, discovery client, provenance/deletion handling, conditions and source/copy watches.
4. RBAC/generation/docs/integration. No Deployment CA mounts or trust-manager changes.

Nested API modules require explicit generation package roots or delegated Make targets; parent paths=./... does not prove API deepcopy coverage. Inspect generated pointer copies and CRDs in all modules, including shared types if generation needed. Add pkg/kongutil to Gateway Make test and coverage scopes; parent Admin tests exclude admin/api.

Ginkgo v2/Gomega; envtest for API/manager/index/watch behavior and restricted identities for RBAC. Test strict parsing/bounds/self-signed/wrong-host; API and token TLS servers with distinct CAs reached through one combined bundle without token-header leakage; source rotation/removal/retention; copy UID/collision/no-op; invalid new ref plus changed endpoint leaves the published provider and discovery unchanged; same-origin different base paths count as target moves; Realm A rejects B; new Realm snapshot persists before any request and conflict sends none; legacy TLS upgrade/literal compatibility/no resolved-password persistence; restart and post-Zone manual rotation; active/terminating dependents observing repaired trust on their next reconciliation; stale caller and stale-build gates; refresh after initial-context cancellation; LRU/idle-close/shutdown/concurrent races. Existing factory mocks do not prove TLS correctness. No projected-volume suite needed.

Explicit integration cases: provisioned/latched Zone with committed Realm A rejects IDP B before IDP/Gateway issuer/discovery changes, with both custom and nil trust. A rejected A-to-B move leaves A's bundle and provider unchanged, whether the source was overwritten or a distinct source was staged; the rotation applies once a configuration is accepted, and historical cleanup observes it then.

Cache/watch verification: use envtest with each manager's actual cache options and observe API list/watch requests to assert the ConfigMap label selector is sent server-side, including watch restart/relist. Assert unrelated ConfigMaps never enter these caches, labeled sources/copies do, Admin's existing per-GVK selectors remain intact, and the uncached APIReader still retrieves nonmatching objects that runtime validation rejects. Exercise initial missing/mismatched source labels (block without a copy; `SourceInvalid` and an unchanged provider with an existing copy), label addition recovery, and label removal/change away from "true" producing a delete event that enqueues the Zone even when the event object is nonmatching. Cover invalid copy label/provenance blocking acquisition once the cached trust expires; source mapping by namespace/ref, copy mapping by verified origin rather than label alone, and shared-source mapping without ownership labels. Restricted-identity RBAC checks must confirm clusterwide ConfigMap get/list/watch remains allowed for labeled and unlabeled objects; cache selectors must not be described as narrowed permissions. Re-run audited non-provider ConfigMap paths with filtering enabled.

For verification use module Make targets only: common/common-server/admin/gateway/identity `make build`, `make test`, `make lint` where supported; also `make -C admin/api test`, then `make verify MODULES="common common-server admin gateway identity"`. Verify does not replace tests. Regenerate sibling API artifacts before Admin envtest.

Verification record of the per-source implementation (it predates the Zone bundle revision, whose verification is recorded with that change; logs in `/tmp/opencode/final-provider-ca-review/`):

- `make build`: common, common-server, admin, gateway, identity — all exit 0.
- `make test`: common (14 packages), common-server (194 packages; 24 reported as cached because nothing they depend on changed), admin, gateway, identity, and `make -C admin/api test` — all exit 0 with no failures. Admin covers the shared Realm snapshot classifier table and a test that a deletion observed at a step boundary stops before the next child write.
- `make lint`: common, admin, gateway, identity — `0 issues`. common-server has no standalone lint target.
- `make verify MODULES="common common-server admin gateway identity"` — exit 2. pre-commit (REUSE, gitleaks), tidy, builds and lint pass for every module except common-server golangci-lint, which reports 281 issues, all in files that existed before this feature. None are in `common-server/pkg/client/provider_tls.go` or its test. This failure predates this feature and is not fixed here.
- `git diff --check` on tracked changes and on all new files: no findings.

Zone deletion and writes: before every child-writing pipeline step, and before provider, copy and trust publications, Admin rechecks Zone liveness directly. Once a deletion or a recreated Zone is observed, no further write is started and the reconciliation ends. A deletion that lands between the last check and a write is not prevented, because Kubernetes has no cross-object transaction. Full teardown remains manual, as described below.

## Documentation, rollout and rollback

Publish this document to `docs/design/provider-ca-references.md`; mark the old design superseded without deleting history. Update `docs/docs/admin-journey/environments-and-zones.md`, architecture/admin/gateway/identity pages, affected reference schemas and installation/RBAC/rotation/teardown docs. Add usable sample ConfigMaps and Zone refs; all new files carry SPDX headers.

Deploy additive CRDs/RBAC, then trust-aware Gateway/Identity binaries, then Admin; label source ConfigMaps with `cp.ei.telekom.de/provider-ca: "true"` and configure references last. Document this new source setup requirement in installation, source-reference and rotation instructions; preserve required labels on sources/copies throughout rotation and retained-copy maintenance. Validate actual requests and persisted cleanup snapshots before completing rollout. Rotate with old+new overlap, switch server certificates, then publish withdrawal. Old binaries may ignore refs or overwrite specs/snapshots: downgrade requires removing custom refs only after endpoints work under system trust, preserving additive CRDs/copies; otherwise roll forward. Zone deletion alone neither revokes trust nor cleans remote resources.

## Alternatives and remaining risks

Direct provider-namespace source placement avoids copying but exposes namespace provisioning/retention to administrators; acceptable future simplification, not a second API path initially. Cross-namespace refs duplicate authorization/lifetime dependencies. A global aggregate requires durable contribution accounting and file activation; not selected. The selected Zone aggregate needs neither: its membership is the Zone spec and it is referenced like any other ConfigMap. Automatic GC/post-Zone source sync would add ownership machinery without being required for first delivery.

Risks: source failure retains old trust intentionally; operator ConfigMap reads are broad; copies require manual maintenance after Zone deletion; deleting providers/auth identities prematurely can still prevent cleanup; active operations may finish under old trust; old-binary skew is unsafe; same-name Zone recreation needs deliberate retained-resource migration. None is hidden behind a promise that references or CA bytes alone guarantee remote cleanup.

## Design review outcome

Senior and minimalist reviewers agreed after three challenge rounds. Revisions established coherent connection/trust publication, committed Realm cleanup-target protection, explicit source-wide rotation authority and authoritative cache reads; removed live detachment, lease/drain protocols and additional recovery sweeps. QA and implementation-feasibility reviewers agreed; status-patch in-memory handoff and distinct issuer derivations were clarified. The design, policies and intentional migration restrictions are accepted and implemented, not awaiting approval. Implementation review and final integrated verification are separate from design acceptance; their current status is recorded above.

Implementation underwent independent senior, minimalist, security and test review. Findings were corrected with regression coverage for authoritative HTTPS checks, strict PEM framing, coherent IDP snapshots, staged publication/deletion races, discovery retries, metadata-only recovery and actual OAuth refresh/retained cleanup. The test reviewer agreed on the corrected acceptance cases; final targeted senior, minimalist and security checks agreed after the remaining Identity publication-epoch and Admin stop/classification fixes. Review is static evidence, separate from the Make results above; it does not eliminate documented cross-object races or the existing common-server lint failure.
