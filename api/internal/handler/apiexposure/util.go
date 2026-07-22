// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// setAlreadyExposedConditions sets NotReady and Blocked conditions on the new ApiExposure
// indicating that the API is already exposed by another ApiExposure.
// It will include information about the team and application that owns the existing ApiExposure.
// e.g. 'API is already exposed by Team "team-a" and their Application "app-1"' or
// 'API is already exposed by your Application "app-1"'
func setAlreadyExposedConditions(existing, candidate *apiv1.ApiExposure) {
	sb := strings.Builder{}
	sb.WriteString("API is already exposed ")

	applicationName := existing.GetLabels()[config.BuildLabelKey("application")]

	if existing.Namespace != candidate.Namespace {
		teamName := existing.Namespace // TODO: should probably be a label
		str := fmt.Sprintf("by Team %q and their Application %q", teamName, applicationName)
		sb.WriteString(str)
	} else {
		fmt.Fprintf(&sb, "by your Application %q", applicationName)
	}

	msg := sb.String()

	candidate.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, msg))
	candidate.SetCondition(condition.NewBlockedCondition(msg))
}

// ApiExposureMustNotAlreadyExist ensures that there is no other active ApiExposure with the same base path.
// If there is, it sets appropriate conditions on the new ApiExposure.
func ApiExposureMustNotAlreadyExist(ctx context.Context, candidate *apiv1.ApiExposure) error {
	found, existingApiExp, err := util.FindActiveAPIExposure(ctx, candidate.Spec.ApiBasePath)
	if err != nil {
		return fmt.Errorf("failed to find active ApiExposure for %q: %w", candidate.Spec.ApiBasePath, err)
	}

	if !found {
		// no other active apiExposure found with same basepath
		candidate.Status.Active = true
		return nil
	}

	if types.Equals(existingApiExp, candidate) {
		// the oldest apiExposure is the same as the one we are trying to handle
		candidate.Status.Active = true
	} else {
		// there is already a different apiExposure active with the same BasePathLabelKey
		// the new one will be blocked until the other is deleted
		candidate.Status.Active = false

		setAlreadyExposedConditions(existingApiExp, candidate)
		return nil
	}

	return nil
}

// ApiMustExist checks if there is an active Api corresponding to the given ApiExposure.
// If not, it sets appropriate conditions on the ApiExposure and cleans up owned Routes.
func ApiMustExist(ctx context.Context, apiExp *apiv1.ApiExposure) (*apiv1.Api, error) {
	janitorClient := cclient.ClientFromContextOrDie(ctx)

	found, api, err := util.FindActiveAPI(ctx, apiExp.Spec.ApiBasePath)
	if err != nil {
		return nil, err
	}

	if !found {
		routeList := &gatewayapi.RouteList{}
		// Using ownedByLabel to cleanup all routes that are owned by the ApiExposure
		_, err := janitorClient.Cleanup(ctx, routeList, cclient.OwnedByLabel(apiExp))
		if err != nil {
			return nil, errors.Wrapf(err,
				"failed to cleanup owned routes for ApiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
		}

		apiExp.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet,
			fmt.Sprintf("API %q is not registered. Cannot provision ApiExposure", apiExp.Spec.ApiBasePath)),
		)
		msg := fmt.Sprintf("API %q is not registered. ApiExposure will be automatically processed, when the API is registered", apiExp.Spec.ApiBasePath)
		apiExp.SetCondition(condition.NewBlockedCondition(msg))

		return nil, nil
	}

	if api.Spec.BasePath == apiExp.Spec.ApiBasePath {
		// early return, the api matches the apiExposure
		return api, nil
	}

	// The same API is registered but it has a different case (e.g. /MyApi vs /myapi)

	msg := fmt.Sprintf("API is registered but the case does not match (got=%q, found=%q). "+
		"Please resolve the conflict by changing the BasePath of either the Api or the ApiExposure.",
		apiExp.Spec.ApiBasePath, api.Spec.BasePath)
	apiExp.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, msg))
	apiExp.SetCondition(condition.NewBlockedCondition(msg))

	return nil, nil
}
