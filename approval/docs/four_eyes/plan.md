<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0    
-->

# Implementation Plan: FourEyes Approval Strategy — Semigranted State Integration

## Overview

This plan implements the FourEyes approval strategy by adding `Semigranted` state handling across all layers of the approval domain and enforcing the distinct-decider constraint via webhook validation.

**Estimated scope:** 8 files modified, 1 file added, CRD regeneration, tests

---

## Step 1: CRD type changes — Approval enum + Decision struct extension

**Files:**
- `approval/api/v1/approval_types.go`
- `approval/api/v1/common_types.go`

**Changes:**

### 1a. Add `Semigranted` to Approval CRD enum

**File:** `approval/api/v1/approval_types.go`

- Line 39: Change `+kubebuilder:validation:Enum=Pending;Granted;Rejected;Suspended` to `+kubebuilder:validation:Enum=Pending;Semigranted;Granted;Rejected;Suspended`
- Line 59: Change `+kubebuilder:validation:Enum=Pending;Granted;Rejected;Suspended` to `+kubebuilder:validation:Enum=Pending;Semigranted;Granted;Rejected;Suspended`

**Rationale:** The Approval CRD must accept `Semigranted` as a valid state value. Without this, the Kubernetes API server rejects any Approval with `state: Semigranted`.

### 1b. Extend Decision struct with Timestamp and ResultingState

**File:** `approval/api/v1/common_types.go`

Add two new fields to the `Decision` struct (after the existing `Comment` field):

```go
type Decision struct {
	// Name of the person making the decision
	Name string `json:"name"`

	// Email of the person making the decision
	Email string `json:"email,omitempty"`

	// Comment provided by the person making the decision
	Comment string `json:"comment,omitempty"`

	// Timestamp of when the decision was made
	// +optional
	Timestamp *metav1.Time `json:"timestamp,omitempty"`

	// ResultingState is the state the resource transitioned to as a result of this decision
	// +optional
	// +kubebuilder:validation:Enum=Pending;Semigranted;Granted;Rejected;Suspended;Expired
	ResultingState ApprovalState `json:"resultingState,omitempty"`
}
```

**Rationale:** Aligns the CRD with the existing GraphQL API model (`controlplane-api`). `Timestamp` uses `*metav1.Time` for Kubernetes-idiomatic time handling. `ResultingState` uses `ApprovalState` typed enum for compile-time safety (see Design Decision 6).

**Note:** Requires adding `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"` to the imports in `common_types.go` if not already present.

### 🚧 Gate 1: CRD Regeneration & Verification
- Run `make manifests` inside the `approval/` directory
- Verify that `approval/config/crd/bases/approval.cp.ei.telekom.de_approvals.yaml` now contains `Semigranted` in the state enum
- Verify that both CRD YAML manifests now contain `timestamp` and `resultingState` fields under `decisions` items
- Run `make generate` to ensure deepcopy functions are up to date (especially for the new `*metav1.Time` pointer field)
- Run `quality-check` skill on both modified files

---

## Step 2: Add `NewSemigrantedCondition()`

**File:** `approval/internal/condition/condition.go`

**Changes:**
Add new function:
```go
func NewSemigrantedCondition() metav1.Condition {
    return metav1.Condition{
        Type:    "Approved",
        Status:  metav1.ConditionFalse,
        Reason:  "Semigranted",
        Message: "Request has been partially approved, awaiting second approval",
    }
}
```

**Rationale:** Follows the existing pattern (`NewApprovedCondition`, `NewPendingCondition`, etc.). Status is `False` because the approval is not yet complete.

### 🚧 Gate 2: Quality Check
- Run `quality-check` skill on `approval/internal/condition/condition.go`
- Verify the function signature and return type match existing patterns

---

## Step 3: Add `Semigranted` case to ApprovalRequest Handler

**File:** `approval/internal/handler/approvalrequest/handler.go`

**Changes:**

1. Add `case approvalv1.ApprovalStateSemigranted:` to the switch statement (between `Granted` and `Rejected` cases):
```go
case approvalv1.ApprovalStateSemigranted:
    log.Info("ApprovalRequest has been partially approved")
    approvalReq.SetCondition(approval_condition.NewSemigrantedCondition())
    approvalReq.SetCondition(condition.NewProcessingCondition("Semigranted", "Request partially approved, awaiting second approval"))
    approvalReq.SetCondition(condition.NewNotReadyCondition("Semigranted", "Request has been partially approved"))
```

2. Update `shouldNotifyRequester()` to also return `false` for `Semigranted`:
```go
func shouldNotifyRequester(approvalRequest *approvalv1.ApprovalRequest) bool {
    if approvalRequest.Spec.State == approvalv1.ApprovalStatePending {
        return false
    }
    if approvalRequest.Spec.State == approvalv1.ApprovalStateSemigranted {
        return false
    }
    return true
}
```

**Rationale:** 
- Semigranted sets `Approved=False` (not yet fully approved), `Processing=InProgress` (still in workflow), `Ready=False` (not provisioned yet)
- Requester is not notified on Semigranted per design decision #2

### 🚧 Gate 3: Quality Check + Test Coverage
- Run `quality-check` skill on `approval/internal/handler/approvalrequest/handler.go`
- Run `test-coverage` skill on `approval/internal/handler/approvalrequest/`

---

## Step 4: Add `Semigranted` case to Approval Handler

**File:** `approval/internal/handler/approval/handler.go`

**Changes:**

Add `case approvalv1.ApprovalStateSemigranted:` to the switch statement:
```go
case approvalv1.ApprovalStateSemigranted:
    approval.SetCondition(approval_condition.NewSemigrantedCondition())
    approval.SetCondition(condition.NewProcessingCondition("Semigranted", "Approval partially granted, awaiting second approval"))
    approval.SetCondition(condition.NewNotReadyCondition("Semigranted", "Approval has been partially granted"))
```

**Rationale:** Same condition semantics as the ApprovalRequest handler. This path is used when an existing Approval goes through re-approval (e.g., `Rejected → Semigranted`).

### 🚧 Gate 4: Quality Check + Test Coverage
- Run `quality-check` skill on `approval/internal/handler/approval/handler.go`
- Run `test-coverage` skill on `approval/internal/handler/approval/`

---

## Step 5: Add distinct-decider webhook validation

**Files:**
- `approval/internal/webhook/v1/approval_webhook.go`
- `approval/internal/webhook/v1/approvalrequest_webhook.go`

**Changes (both files — identical logic):**

In `ValidateUpdate()`, after the existing `AvailableTransitions` check, add:
```go
// Enforce distinct deciders for FourEyes strategy
if newObj.Spec.Strategy == approvalv1.ApprovalStrategyFourEyes {
    if newObj.Spec.State == approvalv1.ApprovalStateGranted && 
       oldObj.Spec.State == approvalv1.ApprovalStateSemigranted {
        if err := validateDistinctDeciders(newObj.Spec.Decisions); err != nil {
            return warnings, err
        }
    }
}
```

Add a shared validation helper (in each webhook file, or extracted to a shared package):
```go
func validateDistinctDeciders(decisions []approvalv1.Decision) error {
    if len(decisions) < 2 {
        return apierrors.NewBadRequest(
            "FourEyes strategy requires at least two decisions for granting")
    }
    last := decisions[len(decisions)-1]
    secondLast := decisions[len(decisions)-2]
    if strings.EqualFold(last.Email, secondLast.Email) {
        return apierrors.NewBadRequest(
            "FourEyes strategy requires two distinct deciders (by email)")
    }
    return nil
}
```

**Rationale:** The webhook is the enforcement point — it runs at admission time before the object is persisted. Checking the last two decisions ensures the transition from Semigranted to Granted was made by a different person than the one who transitioned from Pending to Semigranted.

**Note:** For the Approval webhook, the `ValidateUpdate` signature provides `oldObj` as a typed parameter. For the ApprovalRequest webhook, the same is true. We use `oldObj.Spec.State` to check the previous state rather than `status.LastState` because `oldObj` is the currently-stored version.

### 🚧 Gate 5: Quality Check + Test Coverage
- Run `quality-check` skill on both webhook files
- Run `test-coverage` skill on `approval/internal/webhook/v1/`

---

## Step 6: Add FourEyes integration tests

**Files:**
- `approval/internal/controller/approvalrequest_controller_test.go` — Add FourEyes test cases
- `approval/internal/controller/approval_controller_test.go` — Add Semigranted test case
- `approval/internal/webhook/v1/approval_webhook_test.go` — Add validation tests
- `approval/internal/webhook/v1/approvalrequest_webhook_test.go` — Add validation tests

**Test cases to add:**

### ApprovalRequest Controller Tests
1. **FourEyes: Pending → Semigranted** — Verify conditions are set correctly (Approved=False/Semigranted, Processing=InProgress, Ready=False)
2. **FourEyes: Semigranted → Granted** — Verify Approval object is created with Granted state, conditions are correct
3. **FourEyes: Semigranted → Rejected** — Verify conditions reflect rejection
4. **FourEyes: Pending → Semigranted does not notify requester** — Verify only decider notification is sent

### Approval Controller Tests
1. **Semigranted state** — Verify conditions are set correctly when Approval is in Semigranted state

### Webhook Tests
1. **FourEyes: Allow Semigranted → Granted with distinct deciders** — Should pass
2. **FourEyes: Reject Semigranted → Granted with same decider** — Should fail with error
3. **FourEyes: Reject Semigranted → Granted with < 2 decisions** — Should fail with error
4. **Simple: No distinct-decider enforcement** — Should pass without decisions check

### 🚧 Gate 6: Full Test Suite
- Run `make test` inside the `approval/` directory
- All tests must pass
- Run `test-coverage` skill to verify adequate coverage

---

## Step 7: Add FourEyes sample CR

**File:** `approval/config/samples/approval_v1_approvalrequest_foureyes.yaml` (new)

**Content:** A sample ApprovalRequest with `strategy: FourEyes` and `state: Pending`

### 🚧 Gate 7: Final Verification
- Run `make manifests generate` to ensure all generated files are consistent
- Run `make test` — all tests pass
- Run `make build` (if available) — project compiles
- Review all changed files for correctness

---

## Summary of Changes

| Step | Files | Type |
|------|-------|------|
| 1 | `approval/api/v1/approval_types.go` | Modify (2 lines — add `Semigranted` to enums) |
| 1 | `approval/api/v1/common_types.go` | Modify (add `Timestamp` and `ResultingState` to `Decision` struct) |
| 1 | `approval/config/crd/bases/...approvals.yaml` | Regenerate |
| 2 | `approval/internal/condition/condition.go` | Modify (add function) |
| 3 | `approval/internal/handler/approvalrequest/handler.go` | Modify (add case + update notification) |
| 4 | `approval/internal/handler/approval/handler.go` | Modify (add case) |
| 5 | `approval/internal/webhook/v1/approval_webhook.go` | Modify (add validation) |
| 5 | `approval/internal/webhook/v1/approvalrequest_webhook.go` | Modify (add validation) |
| 6 | Multiple `*_test.go` files | Modify (add test cases) |
| 7 | `approval/config/samples/approval_v1_approvalrequest_foureyes.yaml` | New file |

## Dependencies

- Steps 1 and 2 have no dependencies and can be done in parallel
- Steps 3 and 4 depend on Step 2 (condition function)
- Step 5 depends on Step 1 (CRD must accept Semigranted)
- Step 6 depends on Steps 1–5
- Step 7 has no dependencies
