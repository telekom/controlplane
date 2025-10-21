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
	legacyStrategy := legacyApproval.Spec.Strategy
	log.Info("Fetched legacy approval successfully",
		"legacyApprovalName", legacyApprovalName,
		"legacyState", legacyState,
		"legacyStrategy", legacyStrategy)

	// Special handling for Auto strategy ApprovalRequests
	if approvalRequest.Spec.Strategy == approvalv1.ApprovalStrategyAuto {
		return h.handleAutoStrategy(ctx, approvalRequest, legacyApproval, legacyApprovalName)
	}

	// Normal migration for non-Auto strategies
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

// handleAutoStrategy handles migration for ApprovalRequests with Auto strategy
// If the legacy Approval has Strategy=Auto AND State=Suspended, set the corresponding Approval to State=Rejected
func (h *MigrationHandler) handleAutoStrategy(
	ctx context.Context,
	approvalRequest *approvalv1.ApprovalRequest,
	legacyApproval *approvalv1.Approval,
	legacyApprovalName string,
) error {
	log := log.FromContext(ctx)

	legacyStrategy := legacyApproval.Spec.Strategy
	legacyState := legacyApproval.Spec.State

	log.Info("Handling Auto strategy ApprovalRequest",
		"legacyApprovalName", legacyApprovalName,
		"legacyStrategy", legacyStrategy,
		"legacyState", legacyState,
		"approvalRequestState", approvalRequest.Spec.State)

	// Check if legacy Approval has Strategy=Auto AND State=Suspended
	if legacyStrategy == approvalv1.ApprovalStrategyAuto && legacyState == approvalv1.ApprovalStateSuspended {
		// Auto strategy ApprovalRequests automatically create an Approval with State=Granted
		// We need to find and update that Approval to State=Rejected

		// Get the Approval name from the ApprovalRequest status
		if approvalRequest.Status.Approval.Name == "" {
			log.Info("Approval not created yet (no name in status), will retry on next reconciliation")
			return nil
		}

		approvalName := approvalRequest.Status.Approval.Name
		approvalNamespace := approvalRequest.Namespace

		log.Info("Legacy Approval is Auto+Suspended, looking for corresponding Approval to set to Rejected",
			"approvalName", approvalName,
			"approvalNamespace", approvalNamespace,
			"legacyApprovalName", legacyApprovalName)

		// Fetch the Approval
		approval := &approvalv1.Approval{}
		if err := h.Client.Get(ctx, ctrlclient.ObjectKey{
			Name:      approvalName,
			Namespace: approvalNamespace,
		}, approval); err != nil {
			// Approval not found - might not have been created yet
			if ctrlclient.IgnoreNotFound(err) == nil {
				log.Info("Approval not found yet, will retry on next reconciliation",
					"approvalName", approvalName)
				return nil
			}
			log.Error(err, "Failed to get Approval")
			return errors.Wrap(err, "failed to get approval")
		}

		targetState := approvalv1.ApprovalStateRejected

		// Check if already in the target state
		if approval.Spec.State == targetState {
			log.Info("Approval already in target state, skipping update",
				"currentState", approval.Spec.State,
				"targetState", targetState)
			return nil
		}

		log.Info("Setting Approval to Rejected",
			"approvalName", approvalName,
			"oldState", approval.Spec.State,
			"newState", targetState)

		// Update the state
		approval.Spec.State = targetState

		// Set annotation to track migration
		if approval.Annotations == nil {
			approval.Annotations = make(map[string]string)
		}
		approval.Annotations["migration.cp.ei.telekom.de/last-migrated-state"] = string(targetState)
		approval.Annotations["migration.cp.ei.telekom.de/reason"] = "Auto strategy with Suspended state in legacy"
		approval.Annotations["migration.cp.ei.telekom.de/legacy-approval"] = legacyApprovalName

		// Update the Approval
		if err := h.Client.Update(ctx, approval); err != nil {
			log.Error(err, "Failed to update Approval to Rejected")
			return errors.Wrap(err, "failed to update approval")
		}

		log.Info("Successfully set Auto strategy Approval to Rejected",
			"approvalName", approvalName)
		return nil
	}

	// Legacy Approval is not Auto+Suspended, no migration needed
	log.Info("Legacy Approval is not Auto+Suspended, skipping migration for Auto strategy ApprovalRequest",
		"legacyStrategy", legacyStrategy,
		"legacyState", legacyState)

	return nil
}

// computeLegacyApprovalName determines the legacy Approval name from owner references
func (h *MigrationHandler) computeLegacyApprovalName(approvalRequest *approvalv1.ApprovalRequest) (string, error) {
	// The legacy Approval name format:
	// apisubscription--<api-name>--<rover-name>
	// Example: apisubscription--eni-manual-tests-echo-token-request-test-v1--manual-tests-consumer-token-request

	if len(approvalRequest.OwnerReferences) == 0 {
		return "", nil
	}

	// Get the first owner reference
	owner := approvalRequest.OwnerReferences[0]

	// Convert kind to lowercase for legacy naming convention
	kind := strings.ToLower(owner.Kind)

	// The owner name in new cluster might be: rover-name--api-name
	// But legacy expects: api-name--rover-name
	// Need to swap if it contains "--"
	ownerName := owner.Name
	parts := strings.SplitN(ownerName, "--", 2)

	var legacyName string
	if len(parts) == 2 {
		// Swap: rover-name--api-name becomes api-name--rover-name
		legacyName = kind + "--" + parts[1] + "--" + parts[0]
		h.Log.V(1).Info("Swapped owner name components for legacy format",
			"ownerName", ownerName,
			"roverName", parts[0],
			"apiName", parts[1],
			"legacyName", legacyName)
	} else {
		// No "--" in owner name, use as-is
		legacyName = kind + "--" + ownerName
		h.Log.V(1).Info("Owner name has no components to swap",
			"ownerName", ownerName,
			"legacyName", legacyName)
	}

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
