// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	api "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
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
		// there is no other api with the same BasePathLabelKey
		return errors.New("no api found")
	}

	// sort the list by creation timestamp and get the oldest one
	sort.Slice(apiList.Items, func(i, j int) bool {
		return apiList.Items[i].CreationTimestamp.Before(&apiList.Items[j].CreationTimestamp)
	})

	api := apiList.Items[0]

	if api.Name == obj.Name && api.Namespace == obj.Namespace {
		// the oldest api is the same as the one we are trying to create
		obj.Status.Active = true
		obj.SetCondition(condition.NewReadyCondition("ApiActive", "Api is active"))
		obj.SetCondition(condition.NewDoneProcessingCondition("Api is processed"))
		log.Info("✅ Api is processed")

	} else {
		// there is already a different api active with the same BasePathLabelKey
		// the new api will be blocked until the other is deleted
		obj.Status.Active = false
		obj.SetCondition(condition.NewNotReadyCondition("ApiNotActive", "Api is not active"))
		obj.SetCondition(condition.NewBlockedCondition(
			"Api is blocked, another Api with the same BasePath is active. " +
				"It will be automatically processed, if the other Api will be deleted.",
		))
		log.Info("❌ Api is blocked, another Api with the same BasePath is already active.")
	}

	return nil
}

func (h *ApiHandler) Delete(ctx context.Context, obj *api.Api) error {
	return nil
}
