// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"

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
		BaseHandler: common.NewBaseHandler("tcp.ei.telekom.de/v1", "Rover", "rovers", 100).WithValidation(common.ValidateObjectName),
	}
	handler.SupportsInfo = true

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
		if exposures == nil {
			delete(spec, "exposures")
		} else {
			exposures, ok := exposures.([]any)
			if !ok {
				return errors.New("invalid exposures format")
			}
			spec["exposures"] = PatchExposures(exposures)
		}
	}

	subscriptions, exist := spec["subscriptions"]
	if exist {
		if subscriptions == nil {
			delete(spec, "subscriptions")
		} else {
			subscriptions, ok := subscriptions.([]any)
			if !ok {
				return errors.New("invalid subscriptions format")
			}
			spec["subscriptions"] = PatchSubscriptions(subscriptions)
		}
	}

	obj.SetContent(spec)
	return nil
}

func PatchExposures(exposures []any) []map[string]any {
	if len(exposures) == 0 {
		return nil
	}
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
		} else if _, exist := exposure["eventType"]; exist {
			exposuresMaps[i]["type"] = "event"
		} // TODO: add more types as needed
		security, exist := exposure["security"]
		if exist {
			PatchSecurity(security)
		}
	}

	return exposuresMaps
}

func PatchSubscriptions(subscriptions []any) []map[string]any {
	if len(subscriptions) == 0 {
		return nil
	}
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
		security, exist := subscription["security"]
		if exist {
			PatchSecurity(security)
		}
	}

	return subscriptionsMaps

}

func PatchSecurity(security any) {
	if security == nil {
		return
	}
	securityMap, ok := security.(map[string]any)
	if !ok {
		return
	}

	// if it already has the type set, return
	if _, exist := securityMap["type"]; exist {
		return
	}

	// check for known security schemes and set the type accordingly
	if basicAuthCfg, exist := securityMap["basicAuth"]; exist {
		basicAuthCfgMap, ok := basicAuthCfg.(map[string]any)
		if !ok {
			return
		}
		delete(securityMap, "basicAuth")
		maps.Copy(securityMap, basicAuthCfgMap)
		securityMap["type"] = "basicAuth"
		return
	}
	if oauth2Cfg, exist := securityMap["oauth2"]; exist {
		oauth2CfgMap, ok := oauth2Cfg.(map[string]any)
		if !ok {
			return
		}
		delete(securityMap, "oauth2")
		maps.Copy(securityMap, oauth2CfgMap)
		securityMap["type"] = "oauth2"
		return
	}
}

func (h *RoverHandler) ResetSecret(ctx context.Context, name string) (clientId string, clientSecret string, err error) {
	token := h.Setup(ctx)
	url := h.GetRequestUrl(token.Group, token.Team, name, "secret")

	resp, err := h.SendRequest(ctx, nil, http.MethodPatch, url)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	err = common.CheckResponseCode(resp, http.StatusOK, http.StatusAccepted)
	if err != nil {
		return "", "", err
	}

	var response map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", "", errors.Wrap(err, "failed to parse response")
	}

	return response["clientId"], response["secret"], nil
}
