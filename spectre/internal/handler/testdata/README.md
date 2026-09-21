# Spectre Upgrade Fixtures

Phase 0 persisted-state snapshots for Spectre Listener approval graphs.
Baseline: `feat/spectre` @ `b2186921` (pre-dual-gate).

## Fixture files

| File | Scenario | Strategy | Approval state |
|------|----------|----------|----------------|
| `same-team-granted.yaml` | Same-team auto-approved, fully provisioned | Auto | Granted |
| `same-team-revoked.yaml` | Same-team auto-approved then suspended | Auto | Suspended (was Granted) |

## Object graph per fixture

Each fixture contains the full Listener/SpectreApp/Approval/AR graph:

1. **Listener** - the owner CR
2. **SpectreApplication** - referenced by `spec.application`
3. **Approval** - named `listener--<listener-name>` (legacy convention), owned by Listener
4. **ApprovalRequest** - named `<listener-name>--<hash>`, owned by Listener

## Symbolic UIDs

YAML `uid:` fields use symbolic names (e.g. `uid-listener-granted`). The loader
test replaces these with API-assigned UIDs after creation, then updates all
cross-references (ownerReferences, spec.target.uid, status refs).

## What is omitted

The following children are not included in these static fixtures:

- **RouteListener** - created by the Listener handler during reconciliation,
  requires gateway CRDs and full handler context
- **Bridge Subscribers** (request/response) - created during reconciliation
- **Publisher** - created by SpectreApplication handler

These resources are dynamically generated during reconciliation and depend on
runtime state (zone placement, realm discovery, route creation). Phase 2
migration tests that need these objects should use envtest with the full
handler, not static fixtures.

## Revoked fixture: cleanup state

The revoked fixture (`same-team-revoked.yaml`) represents a state where:
- The Approval transitioned from Granted to Suspended
- The Listener status reflects AccessDenied
- The SpectreApplication still exists (the handler drains routes/subscribers
  on denial but does not delete the SpectreApplication itself)
- Child routes and subscribers are assumed already cleaned up

## Usage

These are test inputs, not `kubectl apply` manifests. The loader
(`legacy_fixture_test.go`) creates objects in dependency order, remaps UIDs,
and validates cross-references. Do not use these files directly with kubectl.
