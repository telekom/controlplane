// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zoneserviceconfig

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/handler/util"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

const (
	tokenEndpointPath = "protocol/openid-connect/token"
)

var _ handler.Handler[*filev1.ZoneServiceConfig] = &ZoneServiceConfigHandler{}

type ZoneServiceConfigHandler struct{}

func (h *ZoneServiceConfigHandler) CreateOrUpdate(ctx context.Context, obj *filev1.ZoneServiceConfig) error {
	c := cclient.ClientFromContextOrDie(ctx)

	zone, err := getZoneForZoneServiceConfig(ctx, obj)
	if err != nil {
		return err
	}

	if !condition.IsReady(zone) {
		obj.SetCondition(condition.NewNotReadyCondition("ZoneNotReady", "Zone is not ready"))
		obj.SetCondition(condition.NewBlockedCondition("Waiting for Zone to be ready"))
		return nil
	}

	apiClientRef := util.GetChildResourceRef(obj)

	apiClient := &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiClientRef.Name,
			Namespace: apiClientRef.Namespace,
		},
	}

	err = getSFTPAPIClient(ctx, apiClient)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get client: %w", err)
		}

		err = createSFTPAPIClient(ctx, obj, zone, apiClient)
		if err != nil {
			return fmt.Errorf("failed to create SFTP API Client: %w", err)
		}
	}

	_, err = createConsumer(ctx, obj, zone)
	if err != nil {
		return err
	}

	route, err := createManagedRoute(ctx, zone, obj)
	if err != nil {
		return err
	}

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("ChildResourcesNotReady", "One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition("ChildResourcesNotReady", "Waiting for child resources"))
		return nil
	}

	apiEndpoint, err := sftpAPIEndpointFromManagedRoute(route, zone, apiClient)
	if err != nil {
		return err
	}

	childResourceRef := util.GetChildResourceRef(obj)

	sftpConfig := &sftpv1.SFTPServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      childResourceRef.Name,
			Namespace: childResourceRef.Namespace,
		},
	}

	mutator := func() error {
		locErr := controllerutil.SetControllerReference(obj, sftpConfig, c.Scheme())
		if locErr != nil {
			return fmt.Errorf("failed to set controller reference for sftpConfig: %w", locErr)
		}
		sftpConfig.Spec.API = apiEndpoint
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, sftpConfig, mutator); err != nil {
		return fmt.Errorf("failed to create or update SFTPServiceConfig %q: %w", obj.Name, err)
	}

	obj.Status.SFTPServiceConfigRef = types.ObjectRefFromObject(sftpConfig)

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("ChildResourcesNotReady", "One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition("ChildResourcesNotReady", "Waiting for child resources"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition("ZoneServiceConfigProvisioned", "ZoneServiceConfig has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("ZoneServiceConfig has been provisioned"))
	return nil
}

func (h *ZoneServiceConfigHandler) Delete(ctx context.Context, obj *filev1.ZoneServiceConfig) error {
	// TODO: add removing of secrets from secret-manager when deleting the SFTP API Client
	return nil
}

func getZoneForZoneServiceConfig(ctx context.Context, obj *filev1.ZoneServiceConfig) (*adminv1.Zone, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	ref := types.ObjectRef{
		Name:      obj.Name,
		Namespace: obj.Labels[cconfig.EnvironmentLabelKey],
	}

	zone := &adminv1.Zone{}
	if err := c.Get(ctx, ref.K8s(), zone); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, ctrlerrors.BlockedErrorf("Zone %q not found", ref.String())
		}
		return nil, fmt.Errorf("failed to get Zone %q: %w", ref.String(), err)
	}
	return zone, nil
}

func getSFTPAPIClient(ctx context.Context, obj *identityv1.Client) error {
	cc := cclient.ClientFromContextOrDie(ctx)

	err := cc.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	if err != nil {
		return err
	}

	if !condition.IsReady(obj) {
		return fmt.Errorf("SFTP API Client %s/%s is not ready", obj.Namespace, obj.Name)
	}

	return nil
}

// createOrUpdateSFTPAPIClient creates or updates an identity Client for the SFTP API.
func createSFTPAPIClient(ctx context.Context, obj *filev1.ZoneServiceConfig, zone *adminv1.Zone, apiClient *identityv1.Client) error {
	if zone.Status.InternalIdentityRealm == nil {
		return ctrlerrors.BlockedErrorf("zone %q has no internal identity realm", zone.Name)
	}

	cc := cclient.ClientFromContextOrDie(ctx)

	clientSecretPath := fmt.Sprintf("zones/%s/file/clientSecret", zone.Name)

	secretValue, err := secretsapi.GenerateSecret()
	if err != nil {
		return fmt.Errorf("failed to generate secret for SFTP API Client: %w", err)
	}

	options := []secretsapi.OnboardingOption{
		secretsapi.WithMergeStrategy(),
		secretsapi.WithSecretValue(clientSecretPath, secretValue),
	}

	availableSecret, err := secretsapi.API().UpsertEnvironment(ctx, contextutil.EnvFromContextOrDie(ctx), options...)
	if err != nil {
		return fmt.Errorf("failed to onboard secrets for SFTP API Client: %w", err)
	}

	ref, found := secretsapi.FindSecretId(availableSecret, clientSecretPath)
	if !found {
		return fmt.Errorf("failed to find secret ID for SFTP API Client at path %q", clientSecretPath)
	}

	mutator := func() error {
		locErr := controllerutil.SetControllerReference(obj, apiClient, cc.Scheme())
		if locErr != nil {
			return fmt.Errorf("failed to set controller reference for apiClient: %w", locErr)
		}

		apiClient.Labels = util.DomainLabel()

		apiClient.Spec.ClientId = apiClient.Name
		apiClient.Spec.ClientSecret = ref
		apiClient.Spec.Realm = zone.Status.InternalIdentityRealm

		return nil
	}

	if _, err := cc.CreateOrUpdate(ctx, apiClient, mutator); err != nil {
		return fmt.Errorf("failed to create or update identity Client %q: %w", apiClient.Name, err)
	}

	return nil
}

func createConsumer(ctx context.Context, obj *filev1.ZoneServiceConfig, zone *adminv1.Zone) (*gatewayapi.Consumer, error) {
	cc := cclient.ClientFromContextOrDie(ctx)

	consumerRef := util.GetChildResourceRef(obj)

	consumer := &gatewayapi.Consumer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consumerRef.Name,
			Namespace: consumerRef.Namespace,
		},
	}

	mutator := func() error {
		locErr := controllerutil.SetControllerReference(obj, consumer, cc.Scheme())
		if locErr != nil {
			return fmt.Errorf("failed to set controller reference for consumer: %w", locErr)
		}

		consumer.Labels = util.DomainLabel()

		consumer.Spec = gatewayapi.ConsumerSpec{
			Gateway: *zone.Status.Gateway,
			Name:    consumerRef.Name,
		}

		return nil
	}

	_, err := cc.CreateOrUpdate(ctx, consumer, mutator)
	if err != nil {
		return nil, fmt.Errorf("failed to create or update consumer %q: %w", consumer.Name, err)
	}

	return consumer, nil
}

func sftpAPIEndpointFromManagedRoute(route *gatewayapi.Route, zone *adminv1.Zone, apiClient *identityv1.Client) (sftpv1.APIEndpoint, error) {
	if len(route.Spec.Hostnames) == 0 {
		return sftpv1.APIEndpoint{}, fmt.Errorf("route %s doesn't have any hostname", types.ObjectRefFromObject(route).String())
	}

	if len(route.Spec.Paths) == 0 {
		return sftpv1.APIEndpoint{}, fmt.Errorf("route %s doesn't have any path", types.ObjectRefFromObject(route).String())
	}

	endpoint := "https://" + route.Spec.Hostnames[0] + route.Spec.Paths[0]

	tokenEndpoint, err := tokenEndpointFromIssuer(zone.Status.Links.InternalIssuer)
	if err != nil {
		return sftpv1.APIEndpoint{}, err
	}

	return sftpv1.APIEndpoint{
		Endpoint:     endpoint,
		Issuer:       tokenEndpoint,
		ClientID:     apiClient.Spec.ClientId,
		ClientSecret: apiClient.Spec.ClientSecret,
	}, nil
}

func tokenEndpointFromIssuer(rawIssuer string) (string, error) {
	if strings.HasSuffix(strings.TrimRight(rawIssuer, "/"), tokenEndpointPath) {
		return rawIssuer, nil
	}
	tokenEndpoint, err := url.JoinPath(rawIssuer, tokenEndpointPath)
	if err != nil {
		return "", fmt.Errorf("building token endpoint from issuer URL %q: %w", rawIssuer, err)
	}
	return tokenEndpoint, nil
}

// createManagedRoute creates a single gateway route for a managed route configuration.
func createManagedRoute(ctx context.Context, zone *adminv1.Zone, obj *filev1.ZoneServiceConfig) (*gatewayapi.Route, error) {
	cc := cclient.ClientFromContextOrDie(ctx)

	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("managed routes require a default preset but none was found: %s", err)
	}

	routeRef := util.GetChildResourceRef(obj)

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeRef.Name,
			Namespace: routeRef.Namespace,
		},
	}

	routeConfig := &obj.Spec.API

	mutator := func() error {
		locErr := controllerutil.SetControllerReference(obj, route, cc.Scheme())
		if locErr != nil {
			return fmt.Errorf("failed to set controller reference for route: %w", locErr)
		}

		upstreamUrl, locErr := url.Parse(routeConfig.Url)
		if locErr != nil {
			return ctrlerrors.BlockedErrorf("cannot parse upstream url of internal route %s: %s", routeConfig.Url, locErr)
		}

		route.Labels = util.DomainLabel()

		upstream := gatewayapi.Upstream{
			Scheme:   upstreamUrl.Scheme,
			Hostname: upstreamUrl.Hostname(),
			Port:     gatewayapi.GetPortOrDefaultFromScheme(upstreamUrl),
			Path:     upstreamUrl.Path,
		}

		hostnames, paths := preset.ResolveHostnamesAndPaths(routeConfig.Path)

		route.Spec = gatewayapi.RouteSpec{
			Type:        gatewayapi.RouteTypePrimary,
			GatewayRef:  *zone.Status.Gateway,
			Backend:     gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:   hostnames,
			Paths:       paths,
			PassThrough: false,
			Traffic:     gatewayapi.Traffic{},
			Security: gatewayapi.Security{
				DisableAccessControl: false,
				TrustedIssuers:       []string{zone.Status.Links.InternalIssuer},
				RealmName:            zone.Status.InternalIdentityRealm.Name,
				DefaultConsumers:     []string{util.GetChildResourceRef(obj).Name},
			},
		}

		return nil
	}

	_, err = cc.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, fmt.Errorf("failed to create or update Gateway route %s in zone %s: %w", route.GetName(), zone.Name, err)
	}
	return route, nil
}
