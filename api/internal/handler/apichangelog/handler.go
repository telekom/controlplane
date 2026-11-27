// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apichangelog

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

var _ handler.Handler[*apiv1.ApiChangelog] = (*ApiChangelogHandler)(nil)

// ApiChangelogHandler reconciles ApiChangelog resources.
// The Api with the same base path becomes the owner of the changelog, so deleting
// the Api removes the changelog. Deleting the changelog never affects the Api.
type ApiChangelogHandler struct {
	// Client is used for persisting the owner reference, which the scoped client cannot do.
	Client client.Client
}

func (h *ApiChangelogHandler) CreateOrUpdate(ctx context.Context, changelog *apiv1.ApiChangelog) error {
	owner, err := util.EnsureApiOwner(ctx, h.Client, changelog, changelog.Spec.BasePath)
	if err != nil {
		return fmt.Errorf("linking ApiChangelog to its Api: %w", err)
	}

	if owner == nil {
		log.FromContext(ctx).V(1).Info("No Api found for changelog", "basePath", changelog.Spec.BasePath)
		return ctrlerrors.BlockedErrorf("no Api with base path %q exists, apply its ApiSpecification first", changelog.Spec.BasePath)
	}

	changelog.SetCondition(condition.NewDoneProcessingCondition("ApiChangelog processed"))
	changelog.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "ApiChangelog is ready"))
	return nil
}

func (h *ApiChangelogHandler) Delete(_ context.Context, _ *apiv1.ApiChangelog) error {
	return nil
}
