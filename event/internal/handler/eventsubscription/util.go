// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription

import (
	"context"
	"slices"
	"strings"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
)

// EventVisibilityMustBeValid checks if the visibility of the EventExposure is compatible with the subscription's zone.
func EventVisibilityMustBeValid(ctx context.Context, exposure *eventv1.EventExposure, sub *eventv1.EventSubscription) (bool, error) {
	subZone, err := util.GetZone(ctx, sub.Spec.Zone.K8s())
	if err != nil {
		return false, err
	}

	switch exposure.Spec.Visibility {
	case eventv1.VisibilityWorld:
		// Any subscription is valid for a WORLD exposure
		return true, nil

	case eventv1.VisibilityEnterprise:
		// For an ENTERPRISE exposure, only subscriptions from an enterprise zone are valid
		return strings.EqualFold(string(subZone.Spec.Visibility), string(adminv1.ZoneVisibilityEnterprise)), nil

	case eventv1.VisibilityZone:
		// For a ZONE exposure, only subscriptions from the same zone are valid
		return exposure.Spec.Zone.Equals(&sub.Spec.Zone), nil

	default:
		// If the visibility is unknown, consider it invalid
		return false, nil
	}
}

// EventScopesMustBeValid checks if all scopes configured by the subscribers are actually supported by the exposure.
func EventScopesMustBeValid(ctx context.Context, apiExposure *eventv1.EventExposure, apiSubscription *eventv1.EventSubscription) (bool, error) {
	requestedScopes := apiSubscription.Spec.Scopes
	supportedScopes := make([]string, 0, len(apiExposure.Spec.Scopes))
	for _, scope := range apiExposure.Spec.Scopes {
		supportedScopes = append(supportedScopes, scope.Name)
	}

	for _, scope := range requestedScopes {
		if !slices.Contains(supportedScopes, scope) {
			return false, nil
		}
	}

	return true, nil
}
