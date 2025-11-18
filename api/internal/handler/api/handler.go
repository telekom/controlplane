// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"cmp"
	"context"
	"slices"

	"github.com/pkg/errors"
	api "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*api.Api] = (*ApiHandler)(nil)

type ApiHandler struct{}

func (h *ApiHandler) CreateOrUpdate(ctx context.Context, obj *api.Api) error {
	log := log.FromContext(ctx)

	scopedClient, ok := cclient.ClientFromContext(ctx)
	if !ok {
		return errors.New("client not found in context")
	}

	// get all existing apis with same BasePathLabelKey
	apiList := &api.ApiList{}
	err := scopedClient.List(ctx, apiList, client.MatchingLabels{
		api.BasePathLabelKey: obj.Labels[api.BasePathLabelKey],
	})
	if err != nil {
		return errors.Wrap(err, "failed to list APIs")
	}

	if len(apiList.Items) == 0 {
		// this should never happen as we are processing an existing api
		return ctrlerrors.BlockedErrorf("FATAL: no APIs found with basePath %q", obj.Labels[api.BasePathLabelKey])
	}

	// sort the list by creation timestamp and get the oldest one
	sortByCreationTime := func(a, b api.Api) int {
		c := a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
		if c == 0 {
			// if creation timestamps are equal, sort by namespace to have a deterministic order
			// we cannot use names here as they are derived from the basePath and thus are identical
			return cmp.Compare(a.GetNamespace(), b.GetNamespace())
		}
		return c
	}

	slices.SortStableFunc(apiList.Items, sortByCreationTime)
	api := apiList.Items[0]

	if types.Equals(&api, obj) {
		// the oldest api is the same as the one we are trying to create
		obj.Status.Active = true
		obj.SetCondition(condition.NewReadyCondition("ApiActive", "Api is active"))
		obj.SetCondition(condition.NewDoneProcessingCondition("Api is processed"))
		log.Info("✅ Api is processed")

	} else {
		// there is already a different api active with the same BasePathLabelKey
		// the new api will be blocked until the other is deleted
		obj.Status.Active = false

		if obj.Spec.BasePath == api.Spec.BasePath {
			// The exact same API (case matches)
			obj.SetCondition(condition.NewNotReadyCondition("ApiNotActive", "Api is not active"))
			obj.SetCondition(condition.NewBlockedCondition(
				"Api is blocked, another Api with the same BasePath is active. " +
					"It will be automatically processed, if the other Api will be deleted.",
			))
			log.Info("❌ Api is blocked, another Api with the same BasePath is already active.")

		} else {
			// The same API is exposed but it has a different case (e.g. /MyApi vs /myapi)
			obj.SetCondition(condition.NewNotReadyCondition("ApiNotActiveCaseConflict", "Api is not active due to case conflict"))
			obj.SetCondition(condition.NewBlockedCondition(
				"Api is blocked, another Api with the same BasePath but different case is active. " +
					"Please resolve the conflict by changing the BasePath of one of the Apis.",
			))
			log.Info("❌ Api is blocked, another Api with the same BasePath but different case is already active.")
		}

	}

	return nil
}

func (h *ApiHandler) Delete(ctx context.Context, obj *api.Api) error {
	return nil
}
