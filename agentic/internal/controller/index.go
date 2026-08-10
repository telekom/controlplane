// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// SetupFieldIndexes registers all field indexes required by agentic controllers.
// Must be called before starting the manager.
func SetupFieldIndexes(ctx context.Context, mgr ctrl.Manager) error {
	if err := index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &approvalv1.ApprovalRequest{}); err != nil {
		return fmt.Errorf("setting owner index for ApprovalRequest: %w", err)
	}
	if err := index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &gatewayv1.ConsumeRoute{}); err != nil {
		return fmt.Errorf("setting owner index for ConsumeRoute: %w", err)
	}
	return nil
}
