// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zoneserviceconfig

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/handler/util"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

const (
	zoneLabelName = "zone"

	identityClientNamePrefix = "sftp-api-"
	tokenEndpointPath        = "protocol/openid-connect/token"
)

var _ handler.Handler[*filev1.ZoneServiceConfig] = &ZoneServiceConfigHandler{}

type ZoneServiceConfigHandler struct{}

func (h *ZoneServiceConfigHandler) CreateOrUpdate(ctx context.Context, obj *filev1.ZoneServiceConfig) error {
	c := cclient.ClientFromContextOrDie(ctx)

	zone, err := getZoneForZoneServiceConfig(ctx, obj)
	if err != nil {
		return err
	}

	apiClient, updated, err := createOrUpdateSFTPAPIClient(ctx, obj, zone)
	if err != nil {
		return err
	}

	conditionReady := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
	if conditionReady != nil && conditionReady.ObservedGeneration == obj.Generation && conditionReady.Status == metav1.ConditionTrue && !updated {
		return nil
	}

	route, err := createManagedRoute(ctx, zone, obj.Spec.API)
	if err != nil {
		return err
	}

	apiEndpoint, err := sftpAPIEndpointFromManagedRoute(route, zone, apiClient)
	if err != nil {
		return err
	}

	sftpConfig := &sftpv1.SFTPServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(obj, sftpConfig, c.Scheme()); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		sftpConfig.Labels = util.ChildLabels(types.ObjectRef{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		})
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
	c := cclient.ClientFromContextOrDie(ctx)

	sftpConfig := &sftpv1.SFTPServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		},
	}
	if err := deleteIfExists(ctx, c, sftpConfig); err != nil {
		return fmt.Errorf("failed to delete SFTPServiceConfig %q: %w", obj.Name, err)
	}

	apiClient := &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      identityClientNamePrefix + "--" + obj.Spec.API.Name,
			Namespace: obj.Namespace,
		},
	}
	if err := deleteIfExists(ctx, c, apiClient); err != nil {
		return fmt.Errorf("failed to delete identity Client %q: %w", apiClient.Name, err)
	}

	// TODO: add removing of secrets from secret-manager when deleting the SFTP API Client
	return deleteManagedRoute(ctx, obj)
}

func deleteManagedRoute(ctx context.Context, obj *filev1.ZoneServiceConfig) error {
	c := cclient.ClientFromContextOrDie(ctx)
	zoneRef := types.ObjectRef{
		Name:      obj.Name,
		Namespace: obj.Namespace,
	}

	zone := &adminv1.Zone{}
	if err := c.Get(ctx, zoneRef.K8s(), zone); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil
		}
		return fmt.Errorf("failed to get Zone %q: %w", zoneRef.String(), err)
	}

	if zone.Status.Gateway == nil {
		return nil
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zone.Status.Gateway.Name + "--" + obj.Spec.API.Name,
			Namespace: zone.Status.Gateway.Namespace,
		},
	}

	if err := deleteIfExists(ctx, c, route); err != nil {
		return fmt.Errorf("failed to delete Gateway Route %q: %w", route.Name, err)
	}

	return nil
}

func deleteIfExists(ctx context.Context, c cclient.ScopedClient, obj client.Object) error {
	if err := c.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil
		}
		return err
	}
	return nil
}

func getZoneForZoneServiceConfig(ctx context.Context, obj *filev1.ZoneServiceConfig) (*adminv1.Zone, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	ref := types.ObjectRef{
		Name:      obj.Name,
		Namespace: obj.Namespace,
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

// createOrUpdateSFTPAPIClient creates or updates an identity Client for the SFTP API.
// It checks if the Client already exists and is ready. If it does not exist or is not ready, it generates a new secret and updates the Client accordingly.
// Return bool value represent if client was updated and needs to be reprocessed.
func createOrUpdateSFTPAPIClient(ctx context.Context, obj *filev1.ZoneServiceConfig, zone *adminv1.Zone) (*identityv1.Client, bool, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	if zone.Status.InternalIdentityRealm == nil {
		return nil, false, ctrlerrors.BlockedErrorf("zone %q has no internal identity realm", zone.Name)
	}

	apiClient := &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      identityClientNamePrefix + "--" + obj.Namespace + "--" + obj.Name,
			Namespace: obj.Namespace,
		},
	}
	err := c.Get(ctx, types.ObjectRefFromObject(apiClient).K8s(), apiClient)
	found := true
	if err != nil {
		if !apierrors.IsNotFound(errors.Cause(err)) {
			return nil, false, fmt.Errorf("failed to get identity Client %q: %w", apiClient.Name, err)
		}

		found = false
	}

	if found {
		if condition.IsReady(apiClient) {
			if apiClient.Status.SecretExpiresAt == nil {
				return apiClient, false, nil
			}

			weekBeforeExp := apiClient.Status.SecretExpiresAt.Add(-7 * 24 * time.Hour)
			if weekBeforeExp.After(time.Now()) {
				return apiClient, false, nil
			}
		} else {
			return nil, false, fmt.Errorf("identity Client %q is not ready", apiClient.Name)
		}
	}

	clientSecretPath := fmt.Sprintf("zones/%s/file/%s/%s/clientSecret", zone.Name, obj.Namespace, obj.Name)

	secretValue, err := secretsapi.GenerateSecret()
	if err != nil {
		return nil, false, fmt.Errorf("failed to generate secret for SFTP API Client: %w", err)
	}

	options := []secretsapi.OnboardingOption{
		secretsapi.WithMergeStrategy(),
		secretsapi.WithSecretValue(clientSecretPath, secretValue),
	}

	availableSecret, err := secretsapi.API().UpsertEnvironment(ctx, zone.Labels[cconfig.EnvironmentLabelKey], options...)
	if err != nil {
		return nil, false, fmt.Errorf("failed to onboard secrets for SFTP API Client: %w", err)
	}

	ref, found := secretsapi.FindSecretId(availableSecret, clientSecretPath)
	if !found {
		return nil, false, ctrlerrors.BlockedErrorf("failed to find secret ID for SFTP API Client at path %q", clientSecretPath)
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(obj, apiClient, c.Scheme()); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		apiClient.Labels = util.ChildLabels(types.ObjectRef{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		})
		apiClient.Spec.ClientId = identityClientNamePrefix + obj.Spec.API.Name
		apiClient.Spec.ClientSecret = ref
		apiClient.Spec.Realm = zone.Status.InternalIdentityRealm.DeepCopy()

		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, apiClient, mutator); err != nil {
		return nil, false, fmt.Errorf("failed to create or update identity Client %q: %w", apiClient.Name, err)
	}

	return apiClient, true, nil
}

func sftpAPIEndpointFromManagedRoute(route *gatewayapi.Route, zone *adminv1.Zone, apiClient *identityv1.Client) (sftpv1.APIEndpoint, error) {
	if len(route.Spec.Hostnames) == 0 || len(route.Spec.Paths) == 0 {
		return sftpv1.APIEndpoint{}, ctrlerrors.BlockedErrorf("managed route URL is required")
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
func createManagedRoute(ctx context.Context, zone *adminv1.Zone, routeConfig adminv1.ManagedRouteConfig) (*gatewayapi.Route, error) {
	cc := cclient.ClientFromContextOrDie(ctx)

	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("managed routes require a default preset but none was found: %s", err)
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zone.Status.Gateway.Name + "--" + routeConfig.Name,
			Namespace: zone.Status.Gateway.Namespace,
		},
	}

	mutator := func() error {
		if route.Labels == nil {
			route.Labels = make(map[string]string)
		}
		route.Labels[cconfig.BuildLabelKey(zoneLabelName)] = zone.Name
		route.Labels[cconfig.OwnerUidLabelKey] = string(zone.GetUID())

		upstreamUrl, locErr := url.Parse(routeConfig.Url)
		if locErr != nil {
			return ctrlerrors.BlockedErrorf("cannot parse upstream url of internal route %s: %s", routeConfig.Url, locErr)
		}

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
			},
		}

		return nil
	}

	_, err = cc.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update Gateway route %s in zone %s: %s", route.GetName(), zone.Name, err)
	}
	return route, nil
}
