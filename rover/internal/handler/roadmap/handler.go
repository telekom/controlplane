// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package roadmap

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ handler.Handler[*roverv1.Roadmap] = (*RoadmapHandler)(nil)

type RoadmapHandler struct{}

// CreateOrUpdate handles the reconciliation of Roadmap resources
// For Roadmap, this is minimal - just set conditions to indicate processing is complete
// No owned resources are created (unlike ApiSpecification which creates Api resources)
func (h *RoadmapHandler) CreateOrUpdate(ctx context.Context, roadmap *roverv1.Roadmap) error {
	// Set conditions to indicate the roadmap is ready
	roadmap.SetCondition(condition.NewDoneProcessingCondition("Roadmap processed"))
	roadmap.SetCondition(condition.NewReadyCondition("Ready", "Roadmap is ready"))

	return nil
}

// Delete handles the deletion of Roadmap resources
// This is a no-op following the ApiSpecification pattern
// File deletion from file-manager is handled by the REST API layer, not the Kubernetes operator
// IMPORTANT: Direct kubectl deletion will orphan files in file-manager
// Always use REST API DELETE endpoint for proper cleanup
func (h *RoadmapHandler) Delete(ctx context.Context, obj *roverv1.Roadmap) error {
	return nil
}
