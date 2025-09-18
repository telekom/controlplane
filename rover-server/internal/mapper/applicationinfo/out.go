// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package applicationinfo

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
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

func MapApplicationInfo(ctx context.Context, rover *roverv1.Rover) (*api.ApplicationInfo, error) {
	if rover == nil {
		return nil, errors.New("input rover is nil")
	}
	appInfo := &api.ApplicationInfo{
		Name: rover.Name,
		Zone: rover.Spec.Zone,
	}

	if err := FillApplicationInfo(ctx, rover, appInfo); err != nil {
		return nil, errors.Wrap(err, "failed to fill application info")
	}
	if err := FillSubscriptionInfo(ctx, rover, appInfo); err != nil {
		return nil, errors.Wrap(err, "failed to fill subscription info")
	}
	if err := FillExposureInfo(ctx, rover, appInfo); err != nil {
		return nil, errors.Wrap(err, "failed to fill exposure info")
	}
	// ... fill other info

	return appInfo, nil
}

func FillApplicationInfo(ctx context.Context, rover *roverv1.Rover, appInfo *api.ApplicationInfo) error {
	appStore := store.ApplicationSecretStore

	if rover == nil || rover.Status.Application == nil {
		return errors.New("rover resource is not processed and does not contain an application")
	}
	if appInfo == nil {
		return errors.New("input applicationInfo is nil")
	}

	app, err := appStore.Get(ctx, rover.Status.Application.Namespace, rover.Status.Application.Name)
	if err != nil {
		WriteStatus(app, appInfo, err)
		return errors.Wrap(err, "failed to get application")
	}

	zoneStore := store.ZoneStore
	zone, err := zoneStore.Get(ctx, rover.Labels[config.EnvironmentLabelKey], rover.Spec.Zone)
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
	appInfo.StargatePublishEventUrl = zone.Status.Links.Url + HorizonPublishEventPathSuffix

	appInfo.Status = status.GetOverallStatus(rover.Status.Conditions)
	return nil
}

func FillSubscriptionInfo(ctx context.Context, rover *roverv1.Rover, appInfo *api.ApplicationInfo) error {
	if rover == nil {
		return errors.New("input rover is nil")
	}
	if appInfo == nil {
		return errors.New("input applicationInfo is nil")
	}

	apiSubStore := store.ApiSubscriptionStore

	appInfo.Subscriptions = make([]api.SubscriptionInfo, len(rover.Status.ApiSubscriptions))
	for i, sub := range rover.Status.ApiSubscriptions {
		apiSub, err := apiSubStore.Get(ctx, sub.Namespace, sub.Name)
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

		appInfo.Subscriptions[i] = subInfo
	}

	return nil
}

func FillExposureInfo(ctx context.Context, rover *roverv1.Rover, appInfo *api.ApplicationInfo) error {
	if rover == nil {
		return errors.New("input rover is nil")
	}
	if appInfo == nil {
		return errors.New("input applicationInfo is nil")
	}

	apiExpStore := store.ApiExposureStore

	appInfo.Exposures = make([]api.ExposureInfo, len(rover.Status.ApiExposures))
	for i, exp := range rover.Status.ApiExposures {
		apiExp, err := apiExpStore.Get(ctx, exp.Namespace, exp.Name)
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

		appInfo.Exposures[i] = expInfo
	}

	return nil
}
