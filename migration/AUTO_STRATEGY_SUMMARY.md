// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

# Auto Strategy Migration - Summary

## Key Insight

**In the new cluster, ApprovalRequests with `Strategy=Auto` are automatically granted and create a corresponding Approval with `State=Granted`.**

Therefore, when we detect a suspended Auto approval in the legacy cluster, we need to update the **Approval** (not the ApprovalRequest) in the new cluster.

## Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│ Legacy Cluster                                                   │
│                                                                  │
│ Approval:                                                        │
│   strategy: Auto                                                 │
│   state: Suspended  ← Policy violation detected!                │
└────────────────────────┬─────────────────────────────────────────┘
                         │
                         │ Migration Operator watches
                         │ ApprovalRequest in new cluster
                         │
┌────────────────────────▼─────────────────────────────────────────┐
│ New Cluster - Before Migration                                   │
│                                                                  │
│ ApprovalRequest:                                                 │
│   name: test-request                                             │
│   strategy: Auto                                                 │
│   state: Granted  ← Always Granted for Auto                     │
│   status:                                                        │
│     approval:                                                    │
│       name: approval-a1b2c3d4e5  ← Reference to Approval        │
│                                                                  │
│ Approval (auto-created):                                         │
│   name: approval-a1b2c3d4e5  ← Different name!                  │
│   strategy: Auto                                                 │
│   state: Granted  ← Auto-created when ApprovalRequest created   │
└────────────────────────┬─────────────────────────────────────────┘
                         │
                         │ Operator detects:
                         │ - ApprovalRequest has Strategy=Auto
                         │ - Legacy Approval is Auto+Suspended
                         │ - Get Approval name from Status
                         │ - Find and update the Approval
                         │
┌────────────────────────▼─────────────────────────────────────────┐
│ New Cluster - After Migration                                    │
│                                                                  │
│ ApprovalRequest:                                                 │
│   name: test-request                                             │
│   strategy: Auto                                                 │
│   state: Granted  ← Unchanged                                   │
│   status:                                                        │
│     approval:                                                    │
│       name: approval-a1b2c3d4e5                                  │
│                                                                  │
│ Approval (UPDATED):                                              │
│   name: approval-a1b2c3d4e5                                      │
│   strategy: Auto                                                 │
│   state: Rejected  ← Changed from Granted to Rejected!          │
│   annotations:                                                   │
│     migration.../last-migrated-state: "Rejected"                │
│     migration.../reason: "Auto strategy with Suspended..."      │
└──────────────────────────────────────────────────────────────────┘
```

## Implementation Details

### What Gets Updated

| Resource | Action |
|----------|--------|
| **ApprovalRequest** | ❌ Not modified (stays Granted) |
| **Approval** | ✅ Updated from Granted → Rejected |

### Handler Logic

```go
func (h *MigrationHandler) handleAutoStrategy(...) error {
    // 1. Check if legacy is Auto+Suspended
    if legacyStrategy == Auto && legacyState == Suspended {
        
        // 2. Get Approval name from ApprovalRequest Status
        if approvalRequest.Status.Approval.Name == "" {
            log.Info("Approval not created yet, will retry")
            return nil
        }
        approvalName := approvalRequest.Status.Approval.Name  // Different name!
        
        // 3. Find the Approval
        approval := &approvalv1.Approval{}
        h.Client.Get(ctx, ObjectKey{
            Name: approvalName,  // From status, not same as ApprovalRequest name!
            Namespace: approvalRequest.Namespace,
        }, approval)
        
        // 4. Update Approval to Rejected
        approval.Spec.State = Rejected
        approval.Annotations["..."] = "..."
        h.Client.Update(ctx, approval)
    }
}
```

### Why This Approach?

1. **ApprovalRequests with Auto strategy are always Granted** in the new cluster
2. **The system auto-creates an Approval** when the ApprovalRequest is created
3. **The Approval has a DIFFERENT name** than the ApprovalRequest
4. **The Approval name is stored in `ApprovalRequest.Status.Approval.Name`**
5. **The Approval is what users interact with** - it's the actual approval decision
6. **If legacy shows suspended**, we need to reject the auto-created Approval
7. **This reflects the policy violation** from the legacy cluster

## Test Cases

### ✅ Test 1: Auto+Suspended → Reject Approval

**Setup:**
- ApprovalRequest: Strategy=Auto, State=Granted
- Approval: Strategy=Auto, State=Granted
- Legacy: Strategy=Auto, State=Suspended

**Result:**
- Approval updated to State=Rejected
- Annotations added

### ✅ Test 2: Already Rejected → Skip

**Setup:**
- Approval: Strategy=Auto, State=Rejected (already)
- Legacy: Strategy=Auto, State=Suspended

**Result:**
- No update (already in target state)

### ✅ Test 3: Auto+Granted → Skip

**Setup:**
- Approval: Strategy=Auto, State=Granted
- Legacy: Strategy=Auto, State=Granted

**Result:**
- No update (no policy violation)

### ✅ Test 4: Non-Auto → Skip

**Setup:**
- Approval: Strategy=Auto, State=Granted
- Legacy: Strategy=Simple, State=Suspended

**Result:**
- No update (not Auto in legacy)

## Monitoring

### Success Log Pattern

```
INFO  Handling Auto strategy ApprovalRequest
INFO  Legacy Approval is Auto+Suspended, looking for corresponding Approval
INFO  Setting Approval to Rejected  approvalName=xxx oldState=Granted newState=Rejected
INFO  Successfully set Auto strategy Approval to Rejected
```

### Skip Log Pattern

```
INFO  Legacy Approval is not Auto+Suspended, skipping migration
```

### Metrics to Watch

```bash
# Check how many Approvals are in Rejected state
kubectl get approvals -A -o json | jq '[.items[] | select(.spec.strategy=="Auto" and .spec.state=="Rejected")] | length'

# Check migration annotations
kubectl get approvals -A -o json | jq '.items[] | select(.metadata.annotations["migration.cp.ei.telekom.de/reason"] != null) | {name: .metadata.name, reason: .metadata.annotations["migration.cp.ei.telekom.de/reason"]}'
```

## Key Takeaways

1. ✅ **Auto ApprovalRequests are always Granted** - this is by design
2. ✅ **Auto ApprovalRequests create Approvals** - this happens automatically
3. ✅ **Approval name ≠ ApprovalRequest name** - they have different names!
4. ✅ **Approval name is in the Status** - stored in `ApprovalRequest.Status.Approval.Name`
5. ✅ **We update the Approval, not the ApprovalRequest** - the Approval is what matters
6. ✅ **Only Auto+Suspended combinations trigger updates** - all other cases are skipped
7. ✅ **Annotations track the migration** - for debugging and auditing

---

**Date:** 2025-01-20  
**Version:** 2.1 (Updated to use ApprovalRequest.Status.Approval.Name to find the Approval)
