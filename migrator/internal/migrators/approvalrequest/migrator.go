// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"time"

	"github.com/pkg/errors"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/migrator/pkg/framework"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ framework.ResourceMigrator = &ApprovalRequestMigrator{}

// ApprovalRequestMigrator orchestrates migration of ApprovalRequest resources
// It implements the ResourceMigrator interface and delegates business logic to the handler
type ApprovalRequestMigrator struct {
	handler *MigrationHandler
}

// NewApprovalRequestMigrator creates a new ApprovalRequest migrator
func NewApprovalRequestMigrator() *ApprovalRequestMigrator {
	mapper := NewApprovalMapper()
	handler := NewMigrationHandler(mapper, ctrl.Log.WithName("handler").WithName("ApprovalRequest"))

	return &ApprovalRequestMigrator{
		handler: handler,
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
// Delegates to handler for business logic
func (m *ApprovalRequestMigrator) ComputeLegacyIdentifier(
	ctx context.Context,
	obj client.Object,
) (namespace, name string, skip bool, err error) {
	approvalRequest, ok := obj.(*approvalv1.ApprovalRequest)
	if !ok {
		return "", "", true, errors.New("object is not an ApprovalRequest")
	}

	return m.handler.ComputeLegacyIdentifier(ctx, approvalRequest)
}

// FetchFromLegacy fetches the legacy Approval resource from remote cluster
// Delegates to handler for fetching logic
func (m *ApprovalRequestMigrator) FetchFromLegacy(
	ctx context.Context,
	remoteClient client.Client,
	namespace, name string,
) (client.Object, error) {
	return m.handler.FetchFromLegacy(ctx, remoteClient, namespace, name)
}

// HasChanged checks if the legacy resource state differs from current
// Delegates to handler for comparison logic
func (m *ApprovalRequestMigrator) HasChanged(
	ctx context.Context,
	current, legacy client.Object,
) bool {
	approvalRequest := current.(*approvalv1.ApprovalRequest)
	approval := legacy.(*approvalv1.Approval)

	return m.handler.HasChanged(ctx, approvalRequest, approval)
}

// ApplyMigration applies the legacy state to the current resource
// Delegates to handler for migration logic
func (m *ApprovalRequestMigrator) ApplyMigration(
	ctx context.Context,
	current, legacy client.Object,
) error {
	approvalRequest := current.(*approvalv1.ApprovalRequest)
	approval := legacy.(*approvalv1.Approval)

	return m.handler.ApplyMigration(ctx, approvalRequest, approval)
}

// GetRequeueAfter returns the duration to wait before next reconciliation
func (m *ApprovalRequestMigrator) GetRequeueAfter() time.Duration {
	return 30 * time.Second
}
