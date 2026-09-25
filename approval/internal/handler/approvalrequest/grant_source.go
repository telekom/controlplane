// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"fmt"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
)

// loadSoleLiveScopedGrantSource lists all ApprovalRequests in the source
// namespace and validates that the expected source is the only non-terminating
// request for its (target, approvalKey) scope. This prevents ambiguous grant
// materialization when cleanup conflicts leave two live Granted requests
// targeting the same scoped Approval.
//
// The partition includes ALL non-terminating requests regardless of state
// (Pending, Semigranted, Granted, Rejected, etc.) because any live request for
// the same scope represents a naming/cleanup conflict.
func (h *ApprovalRequestHandler) loadSoleLiveScopedGrantSource(
	ctx context.Context,
	expected *approvalv1.ApprovalRequest,
) (*approvalv1.ApprovalRequest, error) {
	if h.Reader == nil {
		return nil, fmt.Errorf("uncached reader is nil")
	}

	var allARs approvalv1.ApprovalRequestList
	if err := h.Reader.List(ctx, &allARs, ctrlclient.InNamespace(expected.Namespace)); err != nil {
		return nil, fmt.Errorf("listing approval requests: %w", err)
	}

	// Partition: items matching (target, approvalKey) scope with no DeletionTimestamp.
	var partition []*approvalv1.ApprovalRequest
	for i := range allARs.Items {
		item := &allARs.Items[i]
		if item.DeletionTimestamp != nil {
			continue
		}
		if !approvalv1.ScopedIdentityMatch(item.Spec.Target, expected.Spec.Target,
			item.Spec.ApprovalKey, expected.Spec.ApprovalKey) {
			continue
		}
		partition = append(partition, item)
	}

	// Validate the expected source is in the partition.
	var found *approvalv1.ApprovalRequest
	for _, item := range partition {
		if item.UID == expected.UID {
			found = item
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("expected source %s (UID %s) not found in live partition", expected.Name, expected.UID)
	}

	// Exactly one non-terminating member required.
	if len(partition) > 1 {
		return nil, fmt.Errorf("ambiguous scoped grant source: %d live requests for target %s/%s key %q",
			len(partition), expected.Spec.Target.Namespace, expected.Spec.Target.Name, expected.Spec.ApprovalKey)
	}

	// The single member must be the expected source and Granted.
	if found.UID != expected.UID {
		return nil, fmt.Errorf("sole live request is %s (UID %s), not expected %s (UID %s)",
			found.Name, found.UID, expected.Name, expected.UID)
	}
	if found.Spec.State != approvalv1.ApprovalStateGranted {
		return nil, fmt.Errorf("sole live request %s is not granted (state: %s)", found.Name, found.Spec.State)
	}

	return found.DeepCopy(), nil
}
