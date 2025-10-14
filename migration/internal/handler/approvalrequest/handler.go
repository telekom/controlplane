// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/migration/internal/mapper"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// RemoteClient interface for fetching approvals from the legacy cluster
type RemoteClient interface {
	GetApproval(ctx context.Context, namespace, name string) (*approvalv1.Approval, error)
	ListApprovals(ctx context.Context, namespace string) (*approvalv1.ApprovalList, error)
}

// MigrationHandler handles the migration logic for ApprovalRequests
type MigrationHandler struct {
	Client       ctrlclient.Client
	RemoteClient RemoteClient
	Mapper       *mapper.ApprovalMapper
	Log          logr.Logger
}

// NewMigrationHandler creates a new migration handler
func NewMigrationHandler(
	client ctrlclient.Client,
	remoteClient RemoteClient,
	mapper *mapper.ApprovalMapper,
	log logr.Logger,
) *MigrationHandler {
	return &MigrationHandler{
		Client:       client,
		RemoteClient: remoteClient,
		Mapper:       mapper,
		Log:          log,
	}
}

// Handle processes an ApprovalRequest and migrates state from legacy cluster
func (h *MigrationHandler) Handle(ctx context.Context, approvalRequest *approvalv1.ApprovalRequest) error {
	log := h.Log.WithValues("approvalRequest", approvalRequest.Name, "namespace", approvalRequest.Namespace)

	// Compute the legacy Approval name from owner references
	legacyApprovalName, err := h.computeLegacyApprovalName(approvalRequest)
	if err != nil {
		log.V(5).Info("Cannot compute legacy approval name, skipping", "error", err.Error())
		return nil
	}

	if legacyApprovalName == "" {
		log.V(5).Info("No legacy approval name found, skipping migration")
		return nil
	}

	// Fetch legacy Approval from remote cluster
	legacyApproval, err := h.RemoteClient.GetApproval(
		ctx,
		approvalRequest.Namespace,
		legacyApprovalName,
	)
	if err != nil {
		// Check if it's a not found error
		if strings.Contains(err.Error(), "not found") {
			log.V(5).Info("No legacy Approval found in remote cluster",
				"legacyApprovalName", legacyApprovalName)
			return nil
		}
		return errors.Wrap(err, "failed to fetch legacy approval")
	}

	legacyState := legacyApproval.Spec.State

	// Check if state has changed since last migration
	if !h.hasStateChanged(approvalRequest, legacyState) {
		log.V(5).Info("Legacy state unchanged, skipping update",
			"legacyState", legacyState)
		return nil
	}

	log.Info("Legacy state changed, migrating",
		"legacyApprovalName", legacyApprovalName,
		"oldState", approvalRequest.Spec.State,
		"newState", legacyState)

	// Map legacy approval to approval request
	if err := h.Mapper.MapApprovalToRequest(ctx, approvalRequest, legacyApproval); err != nil {
		return errors.Wrap(err, "failed to map approval")
	}

	// Update the ApprovalRequest
	if err := h.Client.Update(ctx, approvalRequest); err != nil {
		return errors.Wrap(err, "failed to update approval request")
	}

	log.Info("Successfully migrated state",
		"newState", approvalRequest.Spec.State)

	return nil
}

// computeLegacyApprovalName determines the legacy Approval name from owner references
func (h *MigrationHandler) computeLegacyApprovalName(approvalRequest *approvalv1.ApprovalRequest) (string, error) {
	// The legacy Approval name is typically derived from the owner reference
	// Format: <resource-kind>--<resource-name>
	// Example: apisubscription--my-subscription

	if len(approvalRequest.OwnerReferences) == 0 {
		return "", nil
	}

	// Get the first owner reference
	owner := approvalRequest.OwnerReferences[0]

	// Convert kind to lowercase for legacy naming convention
	kind := strings.ToLower(owner.Kind)

	// Construct legacy approval name
	legacyName := kind + "--" + owner.Name

	return legacyName, nil
}

// hasStateChanged checks if the legacy state differs from the current state
func (h *MigrationHandler) hasStateChanged(approvalRequest *approvalv1.ApprovalRequest, legacyState approvalv1.ApprovalState) bool {
	// Check if the last migrated state annotation exists
	lastMigratedState, ok := approvalRequest.Annotations["migration.cp.ei.telekom.de/last-migrated-state"]
	if !ok {
		// No annotation means this is the first migration
		return true
	}

	// Map the legacy state to handle Suspended -> Rejected
	mappedState := h.Mapper.MapState(legacyState)

	// Compare the mapped state with the last migrated state
	return string(mappedState) != lastMigratedState
}
