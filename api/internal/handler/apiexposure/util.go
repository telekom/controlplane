// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetAlreadyExposedConditions sets NotReady and Blocked conditions on the new ApiExposure
// indicating that the API is already exposed by another ApiExposure.
// It will include information about the team and application that owns the existing ApiExposure.
// e.g. 'API is already exposed by Team "team-a" and their Application "app-1"' or
// 'API is already exposed by your Application "app-1"'
func SetAlreadyExposedConditions(existing, new *apiv1.ApiExposure) {
	sb := strings.Builder{}
	sb.WriteString("API is already exposed ")

	applicationName := existing.GetLabels()[config.BuildLabelKey("application")]

	if existing.Namespace != new.Namespace {
		teamName := existing.Namespace // TODO: should probably be a label
		str := fmt.Sprintf("by Team %q and their Application %q", teamName, applicationName)
		sb.WriteString(str)
	} else {
		sb.WriteString(fmt.Sprintf("by your Application %q", applicationName))
	}

	msg := sb.String()

	new.SetCondition(condition.NewNotReadyCondition("ApiExposureNotActive", msg))
	new.SetCondition(condition.NewBlockedCondition(msg))
}

func ApiExposureMustNotAlreadyExist(ctx context.Context, new *apiv1.ApiExposure) error {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	apiExposureList := &apiv1.ApiExposureList{}
	err := scopedClient.List(ctx, apiExposureList,
		client.MatchingLabels{apiv1.BasePathLabelKey: new.Labels[apiv1.BasePathLabelKey]})
	if err != nil {
		return errors.Wrap(err, "failed to list ApiExposures")
	}

	// sort the list by creation timestamp and get the oldest one
	sort.Slice(apiExposureList.Items, func(i, j int) bool {
		return apiExposureList.Items[i].CreationTimestamp.Before(&apiExposureList.Items[j].CreationTimestamp)
	})
	existingApiExp := apiExposureList.Items[0]

	if types.Equals(&existingApiExp, new) {
		// the oldest apiExposure is the same as the one we are trying to handle
		new.Status.Active = true
	} else {
		// there is already a different apiExposure active with the same BasePathLabelKey
		// the new one will be blocked until the other is deleted
		new.Status.Active = false

		SetAlreadyExposedConditions(&existingApiExp, new)

		return nil
	}

	return nil
}

// ApiMustExist checks if there is an active Api corresponding to the given ApiExposure.
// If not, it sets appropriate conditions on the ApiExposure and cleans up owned Routes.
func ApiMustExist(ctx context.Context, apiExp *apiv1.ApiExposure) (*apiv1.Api, error) {
	janitorClient := cclient.ClientFromContextOrDie(ctx)

	//  get corresponding active api
	apiList := &apiv1.ApiList{}
	err := janitorClient.List(ctx, apiList,
		client.MatchingLabels{apiv1.BasePathLabelKey: labelutil.NormalizeLabelValue(apiExp.Spec.ApiBasePath)},
		client.MatchingFields{"status.active": "true"})
	if err != nil {
		return nil, errors.Wrapf(err,
			"failed to list corresponding APIs for ApiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
	}

	hasActiveAPI := len(apiList.Items) > 0
	var api *apiv1.Api
	for _, a := range apiList.Items {
		if a.Status.Active {
			api = &a
			hasActiveAPI = true
			break
		}
	}

	if !hasActiveAPI {
		routeList := &gatewayapi.RouteList{}
		// Using ownedByLabel to cleanup all routes that are owned by the ApiExposure
		_, err := janitorClient.Cleanup(ctx, routeList, cclient.OwnedByLabel(apiExp))
		if err != nil {
			return nil, errors.Wrapf(err,
				"failed to cleanup owned routes for ApiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
		}

		apiExp.SetCondition(condition.NewNotReadyCondition("NoApi",
			fmt.Sprintf("API %q is not registered. Cannot provision ApiExposure", apiExp.Spec.ApiBasePath)),
		)
		msg := fmt.Sprintf("API %q is not registered. ApiExposure will be automatically processed, when the API is registered", apiExp.Spec.ApiBasePath)
		apiExp.SetCondition(condition.NewBlockedCondition(msg))

		return nil, nil
	}

	// validate if basepathes of the api and apiexposure are really equal
	if api.Spec.BasePath != apiExp.Spec.ApiBasePath {
		return api, errors.Errorf("the ApiExposure basepath: %s does not match the corresponding Api basepath: %s",
			apiExp.Spec.ApiBasePath, api.Spec.BasePath)
	}

	return api, nil
}
