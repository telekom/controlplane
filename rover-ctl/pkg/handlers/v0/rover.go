// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// RoverHandler is a specialized handler for Rover resources
type RoverHandler struct {
	*common.BaseHandler
}

func NewRoverHandlerInstance() *RoverHandler {
	handler := &RoverHandler{
		BaseHandler: common.NewBaseHandler("tcp.ei.telekom.de/v1", "Rover", "rovers", 100),
	}
	handler.BaseHandler.SupportsInfo = true

	handler.AddHook(common.PreRequestHook, PatchRoverRequest)

	return handler
}

func PatchRoverRequest(ctx context.Context, obj types.Object) error {
	content := obj.GetContent()
	spec, ok := content["spec"].(map[string]any)
	if !ok {
		return errors.New("invalid spec")
	}
	exposures, exist := spec["exposures"]
	if exist {
		exposures, ok := exposures.([]any)
		if !ok {
			return errors.New("invalid exposures format")
		}
		spec["exposures"] = PatchExposures(exposures)
	}

	subscriptions, exist := spec["subscriptions"]
	if exist {
		subscriptions, ok := subscriptions.([]any)
		if !ok {
			return errors.New("invalid subscriptions format")
		}
		spec["subscriptions"] = PatchSubscriptions(subscriptions)
	}

	obj.SetContent(spec)
	return nil
}

func PatchExposures(exposures []any) []map[string]any {
	exposuresMaps := make([]map[string]any, len(exposures))
	for i, exposure := range exposures {
		exposureMap, ok := exposure.(map[string]any)
		if !ok {
			continue // Skip if the exposure is not a map
		}
		exposuresMaps[i] = exposureMap
	}
	for i, exposure := range exposuresMaps {
		if _, exist := exposure["basePath"]; exist {
			exposuresMaps[i]["type"] = "api"
		} else if _, exist := exposure["port"]; exist {
			exposuresMaps[i]["type"] = "port"
		}
	}

	return exposuresMaps
}

func PatchSubscriptions(subscriptions []any) []map[string]any {
	subscriptionsMaps := make([]map[string]any, len(subscriptions))
	for i, subscription := range subscriptions {
		subscriptionMap, ok := subscription.(map[string]any)
		if !ok {
			continue // Skip if the subscription is not a map
		}
		subscriptionsMaps[i] = subscriptionMap
	}
	for i, subscription := range subscriptionsMaps {
		if _, exist := subscription["basePath"]; exist {
			subscriptionsMaps[i]["type"] = "api"
		} else if _, exist := subscription["port"]; exist {
			subscriptionsMaps[i]["type"] = "port"
		}
	}

	return subscriptionsMaps

}
