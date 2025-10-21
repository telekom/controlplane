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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// MigrationHandler handles the business logic for ApprovalRequest migration
type MigrationHandler struct {
	mapper *ApprovalMapper
	log    logr.Logger
}

// NewMigrationHandler creates a new handler
func NewMigrationHandler(mapper *ApprovalMapper, log logr.Logger) *MigrationHandler {
	return &MigrationHandler{
		mapper: mapper,
		log:    log,
	}
}

// ComputeLegacyIdentifier computes the legacy namespace and name from an ApprovalRequest
func (h *MigrationHandler) ComputeLegacyIdentifier(
	ctx context.Context,
	approvalRequest *approvalv1.ApprovalRequest,
) (namespace, name string, skip bool, err error) {
	log := log.FromContext(ctx)

	// Skip migration for Auto strategy
	if approvalRequest.Spec.Strategy == approvalv1.ApprovalStrategyAuto {
		log.Info("Skipping migration for Auto strategy approval request",
			"strategy", approvalRequest.Spec.Strategy)
		return "", "", true, nil
	}

	// Compute legacy approval name from owner references
	legacyName, err := h.computeLegacyApprovalName(approvalRequest)
	if err != nil {
		return "", "", true, err
	}

	if legacyName == "" {
		log.Info("No owner reference found, skipping migration",
			"ownerReferences", len(approvalRequest.OwnerReferences))
		return "", "", true, nil
	}

	// Compute legacy namespace
	legacyNamespace := h.computeLegacyNamespace(approvalRequest.Namespace)

	log.Info("Computed legacy identifier",
		"currentNamespace", approvalRequest.Namespace,
		"legacyNamespace", legacyNamespace,
		"legacyName", legacyName)

	return legacyNamespace, legacyName, false, nil
}

// HasChanged checks if the legacy resource state differs from current
func (h *MigrationHandler) HasChanged(
	ctx context.Context,
	approvalRequest *approvalv1.ApprovalRequest,
	approval *approvalv1.Approval,
) bool {
	log := log.FromContext(ctx)

	legacyState := approval.Spec.State

	// Check if the last migrated state annotation exists
	lastMigratedState, ok := approvalRequest.Annotations["migration.cp.ei.telekom.de/last-migrated-state"]
	if !ok {
		log.V(1).Info("No previous migration annotation found, treating as first migration",
			"legacyState", legacyState)
		return true
	}

	// Map the legacy state
	mappedState := h.mapper.MapState(legacyState)

	hasChanged := string(mappedState) != lastMigratedState
	log.V(1).Info("Checking if state has changed",
		"lastMigratedState", lastMigratedState,
		"legacyState", legacyState,
		"mappedState", mappedState,
		"hasChanged", hasChanged)

	return hasChanged
}

// ApplyMigration applies the legacy state to the current resource
func (h *MigrationHandler) ApplyMigration(
	ctx context.Context,
	approvalRequest *approvalv1.ApprovalRequest,
	approval *approvalv1.Approval,
) error {
	log := log.FromContext(ctx)

	oldState := approvalRequest.Spec.State

	if err := h.mapper.MapApprovalToRequest(ctx, approvalRequest, approval); err != nil {
		return errors.Wrap(err, "failed to map approval to request")
	}

	log.Info("Applied migration",
		"oldState", oldState,
		"newState", approvalRequest.Spec.State,
		"legacyState", approval.Spec.State)

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
		h.log.V(1).Info("Swapped owner name components for legacy format",
			"ownerName", ownerName,
			"roverName", parts[0],
			"apiName", parts[1],
			"legacyName", legacyName)
	} else {
		// No "--" in owner name, use as-is
		legacyName = kind + "--" + ownerName
		h.log.V(1).Info("Owner name has no components to swap",
			"ownerName", ownerName,
			"legacyName", legacyName)
	}

	h.log.V(1).Info("Computed legacy approval name from owner reference",
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
		h.log.V(1).Info("Namespace does not contain '--' separator, using as-is",
			"namespace", namespace)
		return namespace
	}

	// Return groupName--teamName (everything after first --)
	legacyNamespace := parts[1]

	h.log.V(1).Info("Stripped environment prefix from namespace",
		"original", namespace,
		"environment", parts[0],
		"legacy", legacyNamespace)

	return legacyNamespace
}

// FetchFromLegacy fetches the legacy Approval resource from remote cluster
func (h *MigrationHandler) FetchFromLegacy(
	ctx context.Context,
	remoteClient client.Client,
	namespace, name string,
) (*approvalv1.Approval, error) {
	approval := &approvalv1.Approval{}
	key := client.ObjectKey{Namespace: namespace, Name: name}

	if err := remoteClient.Get(ctx, key, approval); err != nil {
		return nil, err
	}

	return approval, nil
}
