<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0    
-->

# Design: FourEyes Approval Strategy — Semigranted State Integration

## Context

The approval domain implements approval workflows using finite-state-machines (FSMs). Three strategies exist: `Auto`, `Simple`, and `FourEyes`. While `Auto` and `Simple` are fully implemented, `FourEyes` has its FSM transition tables defined but the runtime handling (handlers, CRD validation, conditions, webhook enforcement) is missing.

The core principle of FourEyes is that **at least two distinct people** must approve a request before it is granted. This is modeled through an intermediate `Semigranted` state.

## Design Decisions

### Decision 1: Approach — Full enforcement (Approach B)

**Chosen:** Implement both the `Semigranted` state handling AND distinct-decider validation in the webhook.

**Rationale:** The entire purpose of FourEyes is dual-person authorization. Without enforcing that the second approval comes from a different person, the state machine alone provides no security guarantee — the same person could approve twice.

**Rejected alternative:** Approach A (state machine only, no identity validation) was rejected because it doesn't enforce the four-eyes security principle.

### Decision 2: Requester notification on Semigranted — No

**Chosen:** Do NOT notify the requester when the ApprovalRequest transitions to `Semigranted`.

**Rationale:** The requester only needs to be informed of final outcomes (Granted, Rejected). The intermediate Semigranted state is an internal approval workflow detail between deciders. The decider is still notified so the second approver knows action is needed.

### Decision 3: Approval object created with Granted state directly

**Chosen:** When the ApprovalRequest reaches `Granted` (after `Semigranted → Granted`), the Approval object is created directly in `Granted` state, bypassing the Approval's own Semigranted FSM step.

**Rationale:** The dual-approval has already been completed at the ApprovalRequest level. Forcing the Approval through its own `Pending → Semigranted → Granted` cycle would duplicate the approval process. The Approval's FourEyes FSM exists for **subsequent lifecycle management** — e.g., if an already-granted Approval is rejected and needs re-approval, it goes through `Rejected → Semigranted → Granted` requiring two people again.

### Decision 4: Distinct decider enforcement via Decisions field

**Chosen:** Enforce identity uniqueness by inspecting the `spec.decisions[]` array in the validating webhook.

Each approval action adds a `Decision` entry with `name` and `email`. When a FourEyes resource transitions from `Semigranted → Granted`, the webhook validates:
1. `decisions` contains at least 2 entries
2. The last two entries have different `email` values

**Rationale:** The `Decisions` field already exists on both `ApprovalRequest` and `Approval` specs and is designed to track who made decisions. Email is the strongest identity signal available (more unique than name).

### Decision 5: Semigranted condition semantics

**Chosen:** `Approved` condition is `Status: False`, `Reason: Semigranted`.

**Rationale:** Semigranted means the request is not yet fully approved. It follows the same pattern as `Pending` (Status: False) since the resource should not be provisioned yet.

### Decision 6: Extend Decision model with Timestamp and ResultingState

**Chosen:** Add `Timestamp` and `ResultingState` fields to the CRD `Decision` struct in `common_types.go` to align with the existing GraphQL API model in `controlplane-api`.

- `Timestamp` — `*metav1.Time` (optional). Records when the decision was made. Uses Kubernetes-native `metav1.Time` which serializes to RFC 3339 strings in JSON, compatible with the GraphQL `*string` representation.
- `ResultingState` — `ApprovalState` (typed enum, optional, with kubebuilder enum validation). Records the state the resource transitioned to as a result of this decision. Uses the typed `ApprovalState` enum rather than `*string` for compile-time safety.

**Rationale:** The GraphQL API (`controlplane-api/internal/resolvers/model/embedded.go`) already has `Timestamp *string` and `ResultingState *string` on its `Decision` struct. The CRD must carry the same information so that the API can read it directly from the CR without deriving it. Using typed Go enums rather than raw strings catches invalid values at compile time and via CRD schema validation.

**GraphQL model reference:**
```go
type Decision struct {
    Name           string  `json:"name"`
    Email          *string `json:"email,omitempty"`
    Comment        *string `json:"comment,omitempty"`
    Timestamp      *string `json:"timestamp,omitempty"`
    ResultingState *string `json:"resultingState,omitempty"`
}
```

**CRD target struct:**
```go
type Decision struct {
    Name           string        `json:"name"`
    Email          string        `json:"email,omitempty"`
    Comment        string        `json:"comment,omitempty"`
    Timestamp      *metav1.Time  `json:"timestamp,omitempty"`
    ResultingState ApprovalState `json:"resultingState,omitempty"`
}
```

## FSM Transition Diagrams

### ApprovalRequest — FourEyes Strategy

```
                    ┌─────────┐
                    │ Pending │
                    └────┬────┘
                   Allow │  │ Deny
                         ▼  │
                ┌─────────────┐  │
                │ Semigranted │  │
                └──────┬──────┘  │
              Allow │  │ Deny    │
                    ▼  │         ▼
              ┌─────────┐  ┌──────────┐
              │ Granted │  │ Rejected │
              └─────────┘  └──────────┘
```

### Approval — FourEyes Strategy

```
                    ┌─────────┐
                    │ Pending │
                    └────┬────┘
                   Allow │  │ Deny
                         ▼  │
   ┌──────────┐  ┌─────────────┐  │
   │ Rejected │◄─┤ Semigranted │  │
   └─────┬────┘  └──────┬──────┘  │
    Allow │        Allow │  │      │
          └──────►       ▼  │      ▼
              ┌─────────┐  ┌──────────┐
              │ Granted ├──► Rejected │
              └────┬────┘  └──────────┘
            Suspend│
                   ▼
            ┌───────────┐
            │ Suspended │
            └─────┬─────┘
            Resume│
                  ▼
            ┌─────────┐
            │ Granted │
            └─────────┘
```

## Components Affected

| Layer | File | Change |
|-------|------|--------|
| **CRD types** | `approval/api/v1/approval_types.go` | Add `Semigranted` to `State` and `LastState` kubebuilder enums |
| **CRD types** | `approval/api/v1/common_types.go` | Extend `Decision` struct with `Timestamp` and `ResultingState` fields |
| **Condition** | `approval/internal/condition/condition.go` | Add `NewSemigrantedCondition()` |
| **ApprovalRequest Handler** | `approval/internal/handler/approvalrequest/handler.go` | Add `Semigranted` case in switch + skip requester notification for Semigranted |
| **Approval Handler** | `approval/internal/handler/approval/handler.go` | Add `Semigranted` case in switch |
| **Approval Webhook** | `approval/internal/webhook/v1/approval_webhook.go` | Add distinct-decider validation for FourEyes `Semigranted → Granted` |
| **ApprovalRequest Webhook** | `approval/internal/webhook/v1/approvalrequest_webhook.go` | Add distinct-decider validation for FourEyes `Semigranted → Granted` |
| **CRD manifests** | `approval/config/crd/bases/*.yaml` | Regenerate via `make manifests` |
| **Tests** | Controller, webhook, handler tests | Add FourEyes-specific test cases |
| **Sample** | `approval/config/samples/` | Add FourEyes ApprovalRequest sample |

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| CRD schema change breaks existing resources | Medium | Additive enum change (adding `Semigranted`) is backward compatible |
| Decision identity spoofing | Low | This is an API-server level concern; RBAC controls who can update CRs |
| Email comparison edge cases (casing, aliases) | Low | Use case-insensitive comparison; document that email must match exactly |
| Builder doesn't handle Semigranted ApprovalRequest | Low | Builder checks ApprovalRequest for `Rejected` only; `Semigranted` correctly falls through to Approval existence check, returning `Pending` |
| Decision struct extension breaks existing CRs | Low | New fields are optional (`omitempty`); existing CRs without them are valid. Adding fields is backward compatible. |
