// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package applicationinfo

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/pkg/store"
)

const (
	IrisTokenEndpointSuffix       = "/protocol/openid-connect/token"
	HorizonPublishEventPathSuffix = "horizon/events/v1"
)

func WriteStatus(obj types.Object, appInfo *api.ApplicationInfo, err error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	ready := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
	appInfo.Errors = append(appInfo.Errors, api.Problem{
		Resource: api.ResourceRef{
			ApiVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
		},
		Message: err.Error(),
		Cause:   ready.Message,
	})

	appInfo.Status = status.CompareAndReturn(appInfo.Status, status.GetOverallStatus(obj.GetConditions()))
}

func MapApplicationInfo(ctx context.Context, rover *roverv1.Rover, stores *store.Stores) (*api.ApplicationInfo, error) {
	if rover == nil {
		return nil, errors.New("input rover is nil")
	}
	appInfo := &api.ApplicationInfo{
		Name: rover.Name,
		Zone: rover.Spec.Zone,
	}

	if err := FillApplicationInfo(ctx, rover, appInfo, stores); err != nil {
		return nil, errors.Wrap(err, "failed to fill application info")
	}
	if err := FillSubscriptionInfo(ctx, rover, appInfo, stores); err != nil {
		return nil, errors.Wrap(err, "failed to fill subscription info")
	}
	if err := FillExposureInfo(ctx, rover, appInfo, stores); err != nil {
		return nil, errors.Wrap(err, "failed to fill exposure info")
	}
	if err := FillChevronInfo(ctx, rover, appInfo, stores); err != nil {
		return nil, errors.Wrap(err, "failed to fill chevron info")
	}

	return appInfo, nil
}

func FillApplicationInfo(ctx context.Context, rover *roverv1.Rover, appInfo *api.ApplicationInfo, stores *store.Stores) error {
	if rover == nil || rover.Status.Application == nil {
		return errors.New("rover resource is not processed and does not contain an application")
	}
	if appInfo == nil {
		return errors.New("input applicationInfo is nil")
	}

	app, err := stores.ApplicationSecretStore.Get(ctx, rover.Status.Application.Namespace, rover.Status.Application.Name)
	if err != nil {
		WriteStatus(app, appInfo, err)
		return errors.Wrap(err, "failed to get application")
	}

	zone, err := stores.ZoneStore.Get(ctx, rover.Labels[config.EnvironmentLabelKey], rover.Spec.Zone)
	if err != nil {
		if zone != nil {
			WriteStatus(zone, appInfo, err)
		}
		return errors.Wrap(err, "failed to get zone")
	}

	appInfo.IrisClientId = app.Status.ClientId
	appInfo.IrisClientSecret = app.Status.ClientSecret
	appInfo.IrisIssuerUrl = zone.Status.Links.Issuer
	appInfo.IrisTokenEndpointUrl = appInfo.IrisIssuerUrl + IrisTokenEndpointSuffix

	appInfo.StargateIssuerUrl = zone.Status.Links.LmsIssuer
	appInfo.StargateUrl = zone.Status.Links.Url

	appInfo.Status = status.GetOverallStatus(rover.Status.Conditions)
	return nil
}

func FillSubscriptionInfo(ctx context.Context, rover *roverv1.Rover, appInfo *api.ApplicationInfo, stores *store.Stores) error {
	if rover == nil {
		return errors.New("input rover is nil")
	}
	if appInfo == nil {
		return errors.New("input applicationInfo is nil")
	}

	totalSubs := len(rover.Status.ApiSubscriptions) + len(rover.Status.EventSubscriptions)
	appInfo.Subscriptions = make([]api.SubscriptionInfo, 0, totalSubs)

	// Map API subscriptions
	for _, sub := range rover.Status.ApiSubscriptions {
		apiSub, err := stores.APISubscriptionStore.Get(ctx, sub.Namespace, sub.Name)
		if err != nil {
			WriteStatus(apiSub, appInfo, err)
			continue
		}

		if err := condition.EnsureReady(apiSub); err != nil {
			WriteStatus(apiSub, appInfo, err)
		}

		subInfo := api.SubscriptionInfo{}
		apiSubInfo := api.ApiSubscriptionInfo{
			BasePath: apiSub.Spec.ApiBasePath,
		}
		if err := subInfo.FromApiSubscriptionInfo(apiSubInfo); err != nil {
			return errors.Wrap(err, "failed to convert api subscription info")
		}

		appInfo.Subscriptions = append(appInfo.Subscriptions, subInfo)
	}

	// Map event subscriptions
	for _, sub := range rover.Status.EventSubscriptions {
		eventSub, err := stores.EventSubscriptionStore.Get(ctx, sub.Namespace, sub.Name)
		if err != nil {
			WriteStatus(eventSub, appInfo, err)
			continue
		}

		if err := condition.EnsureReady(eventSub); err != nil {
			WriteStatus(eventSub, appInfo, err)
		}

		subInfo := api.SubscriptionInfo{}
		eventSubInfo := mapEventSubscriptionInfo(eventSub)
		eventSubInfo.HorizonSubscriptionId = eventSub.Status.SubscriptionId
		eventSubInfo.HorizonSubscriptionUrl = eventSub.Status.URL
		if err := subInfo.FromEventSubscriptionInfo(eventSubInfo); err != nil {
			return errors.Wrap(err, "failed to convert event subscription info")
		}

		appInfo.Subscriptions = append(appInfo.Subscriptions, subInfo)

	}

	return nil
}

func FillExposureInfo(ctx context.Context, rover *roverv1.Rover, appInfo *api.ApplicationInfo, stores *store.Stores) error {
	if rover == nil {
		return errors.New("input rover is nil")
	}
	if appInfo == nil {
		return errors.New("input applicationInfo is nil")
	}

	totalExps := len(rover.Status.ApiExposures) + len(rover.Status.EventExposures)
	appInfo.Exposures = make([]api.ExposureInfo, 0, totalExps)

	if err := fillAPIExposures(ctx, rover, appInfo, stores); err != nil {
		return err
	}
	if err := fillEventExposures(ctx, rover, appInfo, stores); err != nil {
		return err
	}
	if err := fillPublishEventURL(ctx, rover, appInfo, stores); err != nil {
		return err
	}

	return nil
}

// fillAPIExposures fetches each API exposure referenced by the Rover status,
// validates its readiness, and appends it to appInfo.Exposures.
func fillAPIExposures(ctx context.Context, rover *roverv1.Rover, appInfo *api.ApplicationInfo, stores *store.Stores) error {
	for _, exp := range rover.Status.ApiExposures {
		apiExp, err := stores.APIExposureStore.Get(ctx, exp.Namespace, exp.Name)
		if err != nil {
			WriteStatus(apiExp, appInfo, err)
			continue
		}

		if err := condition.EnsureReady(apiExp); err != nil {
			WriteStatus(apiExp, appInfo, err)
		}

		expInfo := api.ExposureInfo{}
		apiExpInfo := api.ApiExposureInfo{
			BasePath:   apiExp.Spec.ApiBasePath,
			Upstream:   apiExp.Spec.Upstreams[0].Url,
			Approval:   api.ApprovalStrategy(apiExp.Spec.Approval.Strategy),
			Visibility: api.Visibility(apiExp.Spec.Visibility),
		}
		if err := expInfo.FromApiExposureInfo(apiExpInfo); err != nil {
			return errors.Wrap(err, "failed to convert api exposure info")
		}

		appInfo.Exposures = append(appInfo.Exposures, expInfo)
	}
	return nil
}

// fillEventExposures fetches each event exposure referenced by the Rover status,
// validates its readiness, and appends it to appInfo.Exposures.
func fillEventExposures(ctx context.Context, rover *roverv1.Rover, appInfo *api.ApplicationInfo, stores *store.Stores) error {
	for _, exp := range rover.Status.EventExposures {
		eventExp, err := stores.EventExposureStore.Get(ctx, exp.Namespace, exp.Name)
		if err != nil {
			WriteStatus(eventExp, appInfo, err)
			continue
		}

		if err := condition.EnsureReady(eventExp); err != nil {
			WriteStatus(eventExp, appInfo, err)
		}

		expInfo := api.ExposureInfo{}
		eventExpInfo := mapEventExposureInfo(eventExp)
		if err := expInfo.FromEventExposureInfo(eventExpInfo); err != nil {
			return errors.Wrap(err, "failed to convert event exposure info")
		}

		appInfo.Exposures = append(appInfo.Exposures, expInfo)
	}
	return nil
}

// fillPublishEventURL resolves and sets the publish event URL on appInfo
// when the Rover has event exposures and the URL is not already populated.
func fillPublishEventURL(ctx context.Context, rover *roverv1.Rover, appInfo *api.ApplicationInfo, stores *store.Stores) error {
	bCtx, ok := security.FromContext(ctx)
	if len(rover.Status.EventExposures) == 0 || !ok {
		return nil
	}

	zone, err := stores.ZoneStore.Get(ctx, bCtx.Environment, rover.Spec.Zone)
	if err != nil {
		return errors.Wrap(err, "failed to get zone")
	}

	if appInfo.StargatePublishEventUrl == "" {
		eventCfg, err := stores.EventConfigStore.Get(ctx, zone.Status.Namespace, bCtx.Environment)
		if err != nil {
			return errors.Wrap(err, "failed to get event config")
		}
		appInfo.StargatePublishEventUrl = eventCfg.Status.PublishURL
	}

	return nil
}

// mapEventSubscriptionInfo maps an event domain EventSubscription to the API's EventSubscriptionInfo.
// Only core identifying fields are mapped, matching the pattern of the API subscription info mapper.
func mapEventSubscriptionInfo(in *eventv1.EventSubscription) api.EventSubscriptionInfo {
	return api.EventSubscriptionInfo{
		EventType:    in.Spec.EventType,
		DeliveryType: string(in.Spec.Delivery.Type),
		PayloadType:  string(in.Spec.Delivery.Payload),
		Type:         "event",
	}
}

// mapEventExposureInfo maps an event domain EventExposure to the API's EventExposureInfo.
// Only core identifying fields are mapped, matching the pattern of the API exposure info mapper.
func mapEventExposureInfo(in *eventv1.EventExposure) api.EventExposureInfo {
	return api.EventExposureInfo{
		EventType:  in.Spec.EventType,
		Visibility: toApiVisibilityFromEvent(in.Spec.Visibility),
		Approval:   toApiApprovalStrategyFromEvent(in.Spec.Approval.Strategy),
		Type:       "event",
	}
}

// toApiVisibilityFromEvent converts event domain Visibility to API Visibility.
func toApiVisibilityFromEvent(visibility eventv1.Visibility) api.Visibility {
	switch visibility {
	case eventv1.VisibilityWorld:
		return api.WORLD
	case eventv1.VisibilityZone:
		return api.ZONE
	case eventv1.VisibilityEnterprise:
		return api.ENTERPRISE
	default:
		return api.Visibility(strings.ToUpper(string(visibility)))
	}
}

// toApiApprovalStrategyFromEvent converts event domain ApprovalStrategy to API ApprovalStrategy.
func toApiApprovalStrategyFromEvent(strategy eventv1.ApprovalStrategy) api.ApprovalStrategy {
	switch strategy {
	case eventv1.ApprovalStrategyAuto:
		return api.AUTO
	case eventv1.ApprovalStrategySimple:
		return api.SIMPLE
	case eventv1.ApprovalStrategyFourEyes:
		return api.FOUREYES
	default:
		return api.ApprovalStrategy(strings.ToUpper(string(strategy)))
	}
}

// FillChevronInfo populates Chevron permission-related fields in ApplicationInfo
// when the Rover has authorization configured.
func FillChevronInfo(ctx context.Context, rover *roverv1.Rover, appInfo *api.ApplicationInfo, stores *store.Stores) error {
	// Only populate chevron info if authorization is configured
	if len(rover.Spec.Authorization) == 0 {
		return nil
	}

	bCtx, ok := security.FromContext(ctx)
	if !ok {
		return nil
	}

	zone, err := stores.ZoneStore.Get(ctx, bCtx.Environment, rover.Spec.Zone)
	if err != nil {
		return errors.Wrap(err, "failed to get zone for chevron info")
	}

	// Chevron URL from zone status links + application query param
	if zone.Status.Links.ChevronUrl != "" {
		appInfo.ChevronUrl = zone.Status.Links.ChevronUrl + "?application=" + appInfo.IrisClientId
		appInfo.ChevronApplication = appInfo.IrisClientId

		// Add variables
		appInfo.Variables = append(appInfo.Variables, api.Data{
			Name:  "tardis.chevron.url",
			Value: appInfo.ChevronUrl,
		})
		appInfo.Variables = append(appInfo.Variables, api.Data{
			Name:  "tardis.chevron.application",
			Value: appInfo.ChevronApplication,
		})

		// Copy authorization rules
		appInfo.Authorization = make([]api.AuthorizationInfo, 0, len(rover.Spec.Authorization))
		for _, auth := range rover.Spec.Authorization {
			authInfo := api.AuthorizationInfo{
				Resource: auth.Resource,
				Role:     auth.Role,
				Actions:  auth.Actions,
			}
			if len(auth.Permissions) > 0 {
				perms := make([]api.AuthorizationPermissionInfo, 0, len(auth.Permissions))
				for _, perm := range auth.Permissions {
					perms = append(perms, api.AuthorizationPermissionInfo{
						Resource: perm.Resource,
						Role:     perm.Role,
						Actions:  perm.Actions,
					})
				}
				authInfo.Permissions = perms
			}
			appInfo.Authorization = append(appInfo.Authorization, authInfo)
		}
	}

	return nil
}
