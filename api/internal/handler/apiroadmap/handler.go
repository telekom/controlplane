// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiroadmap

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
)

var _ handler.Handler[*apiv1.ApiRoadmap] = (*ApiRoadmapHandler)(nil)

// ApiRoadmapHandler reconciles ApiRoadmap resources.
// The Api with the same base path becomes the owner of the roadmap, so deleting
// the Api removes the roadmap. Deleting the roadmap never affects the Api.
type ApiRoadmapHandler struct {
	// Client is used for persisting the owner reference, which the scoped client cannot do.
	Client client.Client
}

func (h *ApiRoadmapHandler) CreateOrUpdate(ctx context.Context, roadmap *apiv1.ApiRoadmap) error {
	owner, err := util.EnsureApiOwner(ctx, h.Client, roadmap, roadmap.Spec.BasePath)
	if err != nil {
		return fmt.Errorf("linking ApiRoadmap to its Api: %w", err)
	}

	if owner == nil {
		log.FromContext(ctx).V(1).Info("No Api found for roadmap", "basePath", roadmap.Spec.BasePath)
		return ctrlerrors.BlockedErrorf("no Api with base path %q exists, apply its ApiSpecification first", roadmap.Spec.BasePath)
	}

	roadmap.SetCondition(condition.NewDoneProcessingCondition("ApiRoadmap processed"))
	roadmap.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "ApiRoadmap is ready"))
	return nil
}

func (h *ApiRoadmapHandler) Delete(_ context.Context, _ *apiv1.ApiRoadmap) error {
	return nil
}
