// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/migrator/pkg/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ framework.ResourceMigrator = &ApprovalRequestMigrator{}

// ApprovalRequestMigrator handles migration of ApprovalRequest resources
type ApprovalRequestMigrator struct {
	mapper *ApprovalMapper
}

// NewApprovalRequestMigrator creates a new ApprovalRequest migrator
func NewApprovalRequestMigrator() *ApprovalRequestMigrator {
	return &ApprovalRequestMigrator{
		mapper: NewApprovalMapper(),
	}
}

// GetName returns the unique name of this migrator
func (m *ApprovalRequestMigrator) GetName() string {
	return "approvalrequest"
}

// GetNewResourceType returns an empty instance of the new cluster resource
func (m *ApprovalRequestMigrator) GetNewResourceType() client.Object {
	return &approvalv1.ApprovalRequest{}
}

// GetLegacyAPIGroup returns the legacy API group
func (m *ApprovalRequestMigrator) GetLegacyAPIGroup() string {
	return "acp.ei.telekom.de"
}

// ComputeLegacyIdentifier computes the legacy namespace and name
func (m *ApprovalRequestMigrator) ComputeLegacyIdentifier(
	ctx context.Context,
	obj client.Object,
) (namespace, name string, skip bool, err error) {
	log := log.FromContext(ctx)

	approvalRequest, ok := obj.(*approvalv1.ApprovalRequest)
	if !ok {
		return "", "", true, errors.New("object is not an ApprovalRequest")
	}

	// Compute legacy approval name from owner references
	legacyName, err := m.computeLegacyApprovalName(approvalRequest)
	if err != nil {
		return "", "", true, err
	}

	if legacyName == "" {
		log.Info("No owner reference found, skipping migration",
			"ownerReferences", len(approvalRequest.OwnerReferences))
		return "", "", true, nil
	}

	// Compute legacy namespace
	legacyNamespace := m.computeLegacyNamespace(approvalRequest.Namespace)

	log.Info("Computed legacy identifier",
		"currentNamespace", approvalRequest.Namespace,
		"legacyNamespace", legacyNamespace,
		"legacyName", legacyName)

	return legacyNamespace, legacyName, false, nil
}

// FetchFromLegacy fetches the legacy Approval resource from remote cluster
func (m *ApprovalRequestMigrator) FetchFromLegacy(
	ctx context.Context,
	remoteClient client.Client,
	namespace, name string,
) (client.Object, error) {
	approval := &approvalv1.Approval{}
	key := client.ObjectKey{Namespace: namespace, Name: name}

	if err := remoteClient.Get(ctx, key, approval); err != nil {
		return nil, err
	}

	return approval, nil
}

// HasChanged checks if the legacy resource state differs from current
func (m *ApprovalRequestMigrator) HasChanged(
	ctx context.Context,
	current, legacy client.Object,
) bool {
	log := log.FromContext(ctx)

	approvalRequest := current.(*approvalv1.ApprovalRequest)
	approval := legacy.(*approvalv1.Approval)

	legacyState := approval.Spec.State

	// Check if the last migrated state annotation exists
	lastMigratedState, ok := approvalRequest.Annotations["migration.cp.ei.telekom.de/last-migrated-state"]
	if !ok {
		log.V(1).Info("No previous migration annotation found, treating as first migration",
			"legacyState", legacyState)
		return true
	}

	// Map the legacy state
	mappedState := m.mapper.MapState(legacyState)

	hasChanged := string(mappedState) != lastMigratedState
	log.V(1).Info("Checking if state has changed",
		"lastMigratedState", lastMigratedState,
		"legacyState", legacyState,
		"mappedState", mappedState,
		"hasChanged", hasChanged)

	return hasChanged
}

// ApplyMigration applies the legacy state to the current resource
func (m *ApprovalRequestMigrator) ApplyMigration(
	ctx context.Context,
	current, legacy client.Object,
) error {
	log := log.FromContext(ctx)

	approvalRequest := current.(*approvalv1.ApprovalRequest)
	approval := legacy.(*approvalv1.Approval)

	oldState := approvalRequest.Spec.State

	if err := m.mapper.MapApprovalToRequest(ctx, approvalRequest, approval); err != nil {
		return err
	}

	log.Info("Applied migration",
		"oldState", oldState,
		"newState", approvalRequest.Spec.State,
		"legacyState", approval.Spec.State)

	return nil
}

// GetRequeueAfter returns the duration to wait before next reconciliation
func (m *ApprovalRequestMigrator) GetRequeueAfter() time.Duration {
	return 30 * time.Second
}

// computeLegacyApprovalName determines the legacy Approval name from owner references
func (m *ApprovalRequestMigrator) computeLegacyApprovalName(approvalRequest *approvalv1.ApprovalRequest) (string, error) {
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
	} else {
		// No "--" in owner name, use as-is
		legacyName = kind + "--" + ownerName
	}

	return legacyName, nil
}

// computeLegacyNamespace strips the environment prefix from the namespace
func (m *ApprovalRequestMigrator) computeLegacyNamespace(namespace string) string {
	// New cluster namespace format: environment--groupName--teamName
	// Legacy cluster namespace format: groupName--teamName
	// Strip the first segment (environment)

	parts := strings.SplitN(namespace, "--", 2)
	if len(parts) < 2 {
		// If no "--" separator found, return as-is (already in legacy format)
		return namespace
	}

	// Return groupName--teamName (everything after first --)
	return parts[1]
}
