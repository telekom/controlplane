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
	"sigs.k8s.io/controller-runtime/pkg/log"
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
	log := log.FromContext(ctx)

	// Compute the legacy Approval name from owner references
	legacyApprovalName, err := h.computeLegacyApprovalName(approvalRequest)
	if err != nil {
		log.Info("Cannot compute legacy approval name, skipping migration", "error", err.Error())
		return nil
	}

	if legacyApprovalName == "" {
		log.Info("No owner reference found, skipping migration",
			"ownerReferences", len(approvalRequest.OwnerReferences))
		return nil
	}

	log.Info("Computed legacy approval name", "legacyApprovalName", legacyApprovalName)

	// Compute legacy namespace by stripping environment prefix
	// New cluster: environment--groupName--teamName (e.g., controlplane--eni--narvi)
	// Legacy cluster: groupName--teamName (e.g., eni--narvi)
	legacyNamespace := h.computeLegacyNamespace(approvalRequest.Namespace)
	
	log.Info("Computed legacy namespace",
		"currentNamespace", approvalRequest.Namespace,
		"legacyNamespace", legacyNamespace)

	// Fetch legacy Approval from remote cluster
	log.Info("Fetching legacy approval from remote cluster",
		"namespace", legacyNamespace,
		"name", legacyApprovalName)
	legacyApproval, err := h.RemoteClient.GetApproval(
		ctx,
		legacyNamespace,
		legacyApprovalName,
	)
	if err != nil {
		// Check if it's a not found error
		if strings.Contains(err.Error(), "not found") {
			log.Info("No legacy Approval found in remote cluster, skipping migration",
				"legacyApprovalName", legacyApprovalName)
			return nil
		}
		log.Error(err, "Failed to fetch legacy approval from remote cluster",
			"legacyApprovalName", legacyApprovalName)
		return errors.Wrap(err, "failed to fetch legacy approval")
	}

	legacyState := legacyApproval.Spec.State
	log.Info("Fetched legacy approval successfully",
		"legacyApprovalName", legacyApprovalName,
		"legacyState", legacyState)

	// Check if state has changed since last migration
	if !h.hasStateChanged(approvalRequest, legacyState) {
		log.Info("Legacy state unchanged, skipping update",
			"currentState", approvalRequest.Spec.State,
			"legacyState", legacyState)
		return nil
	}

	log.Info("Legacy state changed, migrating",
		"legacyApprovalName", legacyApprovalName,
		"oldState", approvalRequest.Spec.State,
		"newState", legacyState)

	// Map legacy approval to approval request
	if err := h.Mapper.MapApprovalToRequest(ctx, approvalRequest, legacyApproval); err != nil {
		log.Error(err, "Failed to map legacy approval to approval request")
		return errors.Wrap(err, "failed to map approval")
	}

	// Update the ApprovalRequest
	if err := h.Client.Update(ctx, approvalRequest); err != nil {
		log.Error(err, "Failed to update approval request")
		return errors.Wrap(err, "failed to update approval request")
	}

	log.Info("Successfully migrated state",
		"newState", approvalRequest.Spec.State,
		"legacyApprovalName", legacyApprovalName)

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

	h.Log.V(1).Info("Computed legacy approval name from owner reference",
		"ownerKind", owner.Kind,
		"ownerName", owner.Name,
		"legacyName", legacyName)

	return legacyName, nil
}

// computeLegacyNamespace strips the environment prefix from the namespace
func (h *MigrationHandler) computeLegacyNamespace(namespace string) string {
	// New cluster namespace format: environment--groupName--teamName
	// Legacy cluster namespace format: groupName--teamName
	// Strip the first segment (environment)
	
	parts := strings.SplitN(namespace, "--", 2)
	if len(parts) < 2 {
		// If no "--" separator found, return as-is (already in legacy format)
		h.Log.V(1).Info("Namespace does not contain '--' separator, using as-is",
			"namespace", namespace)
		return namespace
	}
	
	// Return groupName--teamName (everything after first --)
	legacyNamespace := parts[1]
	
	h.Log.V(1).Info("Stripped environment prefix from namespace",
		"original", namespace,
		"environment", parts[0],
		"legacy", legacyNamespace)
	
	return legacyNamespace
}

// hasStateChanged checks if the legacy state differs from the current state
func (h *MigrationHandler) hasStateChanged(approvalRequest *approvalv1.ApprovalRequest, legacyState approvalv1.ApprovalState) bool {
	// Check if the last migrated state annotation exists
	lastMigratedState, ok := approvalRequest.Annotations["migration.cp.ei.telekom.de/last-migrated-state"]
	if !ok {
		// No annotation means this is the first migration
		h.Log.V(1).Info("No previous migration annotation found, treating as first migration",
			"legacyState", legacyState)
		return true
	}

	// Map the legacy state to handle Suspended -> Rejected
	mappedState := h.Mapper.MapState(legacyState)

	hasChanged := string(mappedState) != lastMigratedState
	h.Log.V(1).Info("Checking if state has changed",
		"lastMigratedState", lastMigratedState,
		"legacyState", legacyState,
		"mappedState", mappedState,
		"hasChanged", hasChanged)

	// Compare the mapped state with the last migrated state
	return hasChanged
}
