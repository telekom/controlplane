// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
)

// cleanupScopedRequests deletes stale ApprovalRequests owned by the given owner,
// partitioned by approvalKey. Only requests in the same partition (matching
// approvalKey) as the desired request are candidates for deletion. Requests in
// other partitions are retained unconditionally.
//
// For the legacy (unscoped) path (approvalKey == ""), this replaces the previous
// JanitorClient.Cleanup call while preserving its semantics: all owner-scoped
// ARs that are not the desired one are deleted, but only those with an empty
// approvalKey — scoped ARs on the same owner are not touched.
func cleanupScopedRequests(
	ctx context.Context,
	c cclient.JanitorClient,
	owner types.Object,
	desired *v1.ApprovalRequest,
	approvalKey string,
) (int, error) {
	logger := log.FromContext(ctx)

	var arList v1.ApprovalRequestList
	listOpts := cclient.OwnedBy(owner)
	if err := c.List(ctx, &arList, listOpts...); err != nil {
		return 0, errors.Wrap(err, "listing owner approval-requests for cleanup")
	}

	deleted := 0
	for i := range arList.Items {
		ar := &arList.Items[i]

		// Verify this AR actually belongs to the owner via controller reference.
		if !isOwnedByUID(ar, owner) {
			continue
		}

		// Determine partition from spec.approvalKey (not the label).
		arKey := ar.Spec.ApprovalKey

		// Only operate within our partition.
		if arKey != approvalKey {
			continue
		}

		// Retain the desired request.
		if ar.UID == desired.UID {
			continue
		}

		logger.V(1).Info("Deleting stale approval-request",
			"name", ar.Name,
			"approvalKey", arKey,
			"desiredName", desired.Name)

		// Snapshot identity before Delete — ScopedClient.Delete re-reads into ar,
		// so taking &ar.UID would match the refreshed object, not the validated one.
		uid := ar.UID
		resourceVersion := ar.ResourceVersion

		delOpts := &client.DeleteOptions{
			Preconditions: &metav1.Preconditions{
				UID:             &uid,
				ResourceVersion: &resourceVersion,
			},
		}
		if err := c.Delete(ctx, ar, delOpts); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			// Conflict means the stale request's identity changed between our
			// list and the delete — the obsolete object may still exist. Surface
			// the error so the caller retries rather than silently leaving a
			// stale request behind.
			return deleted, errors.Wrapf(err, "deleting stale approval-request %s", ar.Name)
		}
		deleted++
	}

	return deleted, nil
}

// isOwnedByUID checks that the AR has a controller owner reference matching the
// owner's UID. This prevents acting on resources that happen to share a name
// index but belong to a different controller.
func isOwnedByUID(ar *v1.ApprovalRequest, owner types.Object) bool {
	for _, ref := range ar.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller && ref.UID == owner.GetUID() {
			return true
		}
	}
	return false
}
