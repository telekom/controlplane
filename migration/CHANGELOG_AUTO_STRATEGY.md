// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

# Auto Strategy Handling - Changelog

## Summary

Updated the migration operator to handle `ApprovalRequest` resources with `strategy: Auto` instead of skipping them.

## Previous Behavior ❌

- **Skipped** all ApprovalRequests with `strategy: Auto`
- Rationale: Auto approvals are automatically granted, so no migration needed
- Issue: Suspended Auto approvals in legacy cluster were not migrated

## New Behavior ✅

- **Processes** ApprovalRequests with `strategy: Auto`
- **Rule**: If legacy Approval has `Strategy=Auto` AND `State=Suspended`, find and update the corresponding **Approval** (not ApprovalRequest) to `State=Rejected`
- **Rationale**: 
  - Auto ApprovalRequests are automatically granted in the new cluster
  - The system auto-creates an Approval resource with `State=Granted`
  - If the legacy cluster shows a suspended Auto approval, we need to reject the auto-created Approval
  - This indicates policy violations that should be explicitly rejected

## Changes Made

### 1. Handler Logic (`internal/handler/approvalrequest/handler.go`)

#### Added New Method: `handleAutoStrategy()`

```go
// handleAutoStrategy handles migration for ApprovalRequests with Auto strategy
// If the legacy Approval has Strategy=Auto AND State=Suspended, set the corresponding Approval to State=Rejected
func (h *MigrationHandler) handleAutoStrategy(
    ctx context.Context,
    approvalRequest *approvalv1.ApprovalRequest,
    legacyApproval *approvalv1.Approval,
    legacyApprovalName string,
) error
```

**Logic:**
1. Check if legacy has `Strategy=Auto` AND `State=Suspended`
2. If yes:
   - Find the corresponding **Approval** resource (same name as ApprovalRequest)
   - Set the Approval to `State=Rejected`
   - Add annotations to track migration
3. If no: Skip migration (no action needed)

#### Modified `Handle()` Method

**Before:**
```go
// Skip migration for Auto strategy
if approvalRequest.Spec.Strategy == approvalv1.ApprovalStrategyAuto {
    log.Info("Skipping migration for Auto strategy approval request")
    return nil
}
```

**After:**
```go
// Special handling for Auto strategy ApprovalRequests
if approvalRequest.Spec.Strategy == approvalv1.ApprovalStrategyAuto {
    return h.handleAutoStrategy(ctx, approvalRequest, legacyApproval, legacyApprovalName)
}
```

### 2. Tests (`internal/handler/approvalrequest/handler_auto_test.go`)

Created comprehensive test suite covering:

- ✅ **Auto+Suspended → Rejected**: Sets state correctly
- ✅ **Already Rejected**: Skips update if already in target state
- ✅ **Auto+Granted**: No migration (already auto-granted)
- ✅ **Auto+Pending**: No migration (not suspended)
- ✅ **Non-Auto+Suspended**: No migration (not Auto strategy)

### 3. Documentation Updates

#### `README.md`
- Updated "Key Features" section
- Updated "Reconciliation Flow" section
- Replaced "Strategy-Based Filtering" with "Auto Strategy Handling"
- Added detailed behavior documentation
- Added example log output
- Added annotation documentation

## Migration Scenarios

### Scenario 1: Auto + Suspended (Action Required) ✅

**Legacy Cluster:**
```yaml
apiVersion: acp.ei.telekom.de/v1
kind: Approval
metadata:
  name: apisubscription--api-name--rover-name
  namespace: eni--hyperion
spec:
  strategy: Auto
  state: Suspended  # Policy violation!
```

**New Cluster (Before Migration):**

ApprovalRequest (always Granted for Auto strategy):
```yaml
apiVersion: approval.cp.ei.telekom.de/v1
kind: ApprovalRequest
metadata:
  name: test-request
  namespace: controlplane--eni--hyperion
spec:
  strategy: Auto
  state: Granted  # Auto requests are always granted
```

Auto-created Approval:
```yaml
apiVersion: approval.cp.ei.telekom.de/v1
kind: Approval
metadata:
  name: test-request  # Same name as ApprovalRequest
  namespace: controlplane--eni--hyperion
spec:
  strategy: Auto
  state: Granted  # Auto-created as granted
```

**New Cluster (After Migration):**

The **Approval** is updated (not the ApprovalRequest):
```yaml
apiVersion: approval.cp.ei.telekom.de/v1
kind: Approval  # This resource is updated!
metadata:
  name: test-request
  namespace: controlplane--eni--hyperion
  annotations:
    migration.cp.ei.telekom.de/last-migrated-state: "Rejected"
    migration.cp.ei.telekom.de/reason: "Auto strategy with Suspended state in legacy"
    migration.cp.ei.telekom.de/legacy-approval: "apisubscription--api-name--rover-name"
spec:
  strategy: Auto
  state: Rejected  # ✅ Updated from Granted to Rejected!
```

### Scenario 2: Auto + Granted (No Action) ✅

**Legacy Cluster:**
```yaml
spec:
  strategy: Auto
  state: Granted  # Normal auto-granted
```

**New Cluster:**
```yaml
spec:
  strategy: Auto
  state: Granted  # ✅ No change needed
```

**Result:** No migration (already auto-granted in both clusters)

### Scenario 3: Simple/FourEyes (Normal Migration) ✅

**Legacy Cluster:**
```yaml
spec:
  strategy: Simple
  state: Granted
```

**New Cluster:**
```yaml
spec:
  strategy: Simple
  state: Pending
```

**Result:** Normal migration applies (full state synchronization)

## Logging

### Auto Strategy Handling Logs

```
INFO  Handling Auto strategy ApprovalRequest  
  legacyApprovalName=apisubscription--api--rover 
  legacyStrategy=Auto 
  legacyState=Suspended 
  approvalRequestState=Granted

INFO  Legacy Approval is Auto+Suspended, looking for corresponding Approval to set to Rejected
  approvalName=test-request
  approvalNamespace=controlplane--eni--hyperion

INFO  Setting Approval to Rejected  
  approvalName=test-request
  oldState=Granted 
  newState=Rejected

INFO  Successfully set Auto strategy Approval to Rejected
  approvalName=test-request
```

### Skip Logs (No Action)

```
INFO  Legacy Approval is not Auto+Suspended, skipping migration for Auto strategy ApprovalRequest  
  legacyStrategy=Auto 
  legacyState=Granted
```

## Annotations Added

When migrating Auto+Suspended to Rejected, annotations are added to the **Approval** (not ApprovalRequest):

```yaml
apiVersion: approval.cp.ei.telekom.de/v1
kind: Approval  # Annotations added to Approval!
metadata:
  name: test-request
  namespace: controlplane--eni--hyperion
  annotations:
    migration.cp.ei.telekom.de/last-migrated-state: "Rejected"
    migration.cp.ei.telekom.de/reason: "Auto strategy with Suspended state in legacy"
    migration.cp.ei.telekom.de/legacy-approval: "apisubscription--api-name--rover-name"
spec:
  state: Rejected
```

## Backward Compatibility

✅ **No breaking changes**
- Existing Simple/FourEyes migrations continue to work
- Auto approvals now handled instead of skipped
- Suspended Auto approvals now properly rejected

## Testing

### Unit Tests
```bash
cd migration
go test ./internal/handler/approvalrequest -v -run TestHandleAutoStrategy
```

### Integration Tests
```bash
make test
```

## Deployment

No configuration changes required. Simply deploy the updated operator:

```bash
kubectl apply -k config/default
kubectl rollout restart deployment migration-operator -n controlplane-system
```

## Monitoring

Watch for log messages:
```bash
kubectl logs -n controlplane-system -l app=migration-operator -f | grep "Auto strategy"
```

Expected patterns:
- `Handling Auto strategy ApprovalRequest` - Processing started
- `Legacy Approval is Auto+Suspended, setting ApprovalRequest to Rejected` - Migration applied
- `Legacy Approval is not Auto+Suspended, skipping migration` - No action needed

---

**Date:** 2025-01-20  
**Author:** Migration Team  
**Issue:** Handle suspended Auto strategy approvals from legacy cluster
