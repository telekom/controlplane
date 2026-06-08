// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	"github.com/telekom/controlplane/admin/internal/handler/util/urls"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
)

const (
	zoneLabelName = "zone"

	// spacegatePathPrefix is the downstream path prefix added to all identity
	// routes (issuer, certs, discovery) when a zone's visibility is World.
	spacegatePathPrefix = "/spacegate"
)

var _ handler.Handler[*adminv1.Zone] = &ZoneHandler{}

type ZoneHandler struct{}

func (h *ZoneHandler) CreateOrUpdate(ctx context.Context, obj *adminv1.Zone) error {
	envName := contextutil.EnvFromContextOrDie(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	environment := &adminv1.Environment{}
	err := c.Get(ctx, client.ObjectKey{Name: envName, Namespace: envName}, environment)
	if err != nil {
		return errors.Wrapf(err, "failed to get environment %s", envName)
	}

	// Namespace
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: strings.ToLower(fmt.Sprintf("%s--%s", environment.Name, obj.Name)),
		},
	}

	mutator := func() error {
		namespace.Labels = map[string]string{
			cconfig.EnvironmentLabelKey:          environment.Name,
			cconfig.BuildLabelKey(zoneLabelName): obj.Name,
		}
		return nil
	}
	_, err = c.CreateOrUpdate(ctx, namespace, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update namespace %s", namespace.Name)
	}

	obj.Status.Namespace = namespace.Name

	handlingContext := HandlingContext{
		Zone:        obj,
		Environment: environment,
		Namespace:   namespace,
	}

	// Identity provider
	identityProvider, err := createIdentityProvider(ctx, handlingContext)
	if err != nil {
		return err
	}
	obj.Status.IdentityProvider = types.ObjectRefFromObject(identityProvider)

	// Identity Realm
	defaultClaims := []identityapi.ClaimConfig{
		{
			Name:  "originZone",
			Value: handlingContext.Zone.Name,
			Type:  identityapi.ClaimTypeHardcodedClaim,
		},
		{
			Name:  "originStargate",
			Value: handlingContext.Zone.Spec.Gateway.Url,
			Type:  identityapi.ClaimTypeHardcodedClaim,
		},
		{
			Name: "clientId",
			Type: identityapi.ClaimTypeSessionNote,
		},
	}
	identityRealm, err := createIdentityRealm(ctx, handlingContext, identityProvider, naming.ForDefaultIdentityRealm(handlingContext.Environment), defaultClaims)
	if err != nil {
		return err
	}
	obj.Status.IdentityRealm = types.ObjectRefFromObject(identityRealm)
	obj.Status.Links.Issuer, err = url.JoinPath(obj.Spec.IdentityProvider.Url, "auth/realms/", identityRealm.Name)
	if err != nil {
		return errors.Wrapf(err, "Cannot combine identityProviderBaseUrl %s with realm name %s", obj.Spec.IdentityProvider.Url, identityRealm.Name)
	}

	// Internal Identity Realm (rover) for admin-config clients
	internalIdentityRealm, err := createIdentityRealm(ctx, handlingContext, identityProvider, naming.ForInternalIdentityRealm(), nil)
	if err != nil {
		return err
	}
	obj.Status.InternalIdentityRealm = types.ObjectRefFromObject(internalIdentityRealm)

	// Identity Client for gateway
	// TBD - how to handle passwords for this client - will be regenerated with every reconciliation
	gatewayClient, err := createIdentityClient(ctx, handlingContext, identityRealm)
	if err != nil {
		return err
	}
	obj.Status.GatewayClient = types.ObjectRefFromObject(gatewayClient)

	// Gateway
	gateway, err := createGateway(ctx, handlingContext)
	if err != nil {
		return err
	}
	obj.Status.Gateway = types.ObjectRefFromObject(gateway)

	// Gateway realm
	gatewayRealm, err := createGatewayRealm(ctx, handlingContext, gateway, naming.ForDefaultGatewayRealm(handlingContext.Environment))
	if err != nil {
		return err
	}
	obj.Status.GatewayRealm = types.ObjectRefFromObject(gatewayRealm)
	obj.Status.Links.Url = obj.Spec.Gateway.Url
	obj.Status.Links.LmsIssuer, err = url.JoinPath(obj.Spec.Gateway.Url, "auth/realms/", gatewayRealm.Name)
	if err != nil {
		return errors.Wrapf(err, "Cannot combine gatewayUrl %s with realm name %s", obj.Spec.Gateway.Url, gatewayRealm.Name)
	}

	// Gateway consumer
	gatewayConsumer, err := createGatewayConsumer(ctx, handlingContext, gatewayRealm)
	if err != nil {
		return err
	}
	obj.Status.GatewayConsumer = types.ObjectRefFromObject(gatewayConsumer)

	// Internal routes configuration
	if obj.Spec.ManagedRoutes != nil {
		if err := reconcileManagedRoutes(ctx, handlingContext, obj, identityProvider, gateway, gatewayRealm); err != nil {
			return err
		}
	} else {
		obj.Status.TeamApiIdentityRealm = nil
		obj.Status.TeamApiGatewayRealm = nil
		obj.Status.ManagedRoutes = nil
		obj.Status.Links.TeamIssuer = ""
	}

	// Cleanup managed routes that were not created or updated during this reconciliation.
	// Using OwnedByLabel because routes live in a different namespace than the Zone CR.
	if _, err := c.Cleanup(ctx, &gatewayapi.RouteList{}, cclient.OwnedByLabel(obj)); err != nil {
		return errors.Wrapf(err, "failed to cleanup stale managed routes for zone %s", obj.Name)
	}

	// Populate Permissions URL if configured and feature enabled
	if cconfig.FeaturePermission.IsEnabled() && obj.Spec.Permissions != nil {
		// Use url.JoinPath to properly handle slashes when combining gateway URL with ApiBasePath
		permissionsUrl, err := url.JoinPath(obj.Status.Links.Url, obj.Spec.Permissions.ApiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to build permissions URL")
		}
		obj.Status.Links.PermissionsUrl = permissionsUrl
	} else {
		obj.Status.Links.PermissionsUrl = ""
	}

	obj.SetCondition(condition.NewReadyCondition("ZoneProvisioned", "Zone has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Zone has been provisioned"))

	return nil
}

func reconcileManagedRoutes(ctx context.Context, handlingContext HandlingContext, zone *adminv1.Zone, identityProvider *identityapi.IdentityProvider, gateway *gatewayapi.Gateway, defaultGatewayRealm *gatewayapi.Realm) error {
	// Reset status to avoid stale/duplicate entries across reconciliations
	zone.Status.ManagedRoutes = nil

	// Partition routes by type
	var teamAPIRoutes, proxyRoutes []adminv1.ManagedRouteConfig
	for _, r := range zone.Spec.ManagedRoutes.Routes {
		switch r.Type {
		case adminv1.ManagedRouteTypeTeamAPI:
			teamAPIRoutes = append(teamAPIRoutes, r)
		case adminv1.ManagedRouteTypeProxy:
			proxyRoutes = append(proxyRoutes, r)
		default:
			return fmt.Errorf("unsupported managed route type %q for route %q", r.Type, r.Name)
		}
	}

	// TeamAPI routes require a dedicated identity and gateway realm
	if err := reconcileTeamAPIRoutes(ctx, handlingContext, zone, identityProvider, gateway, teamAPIRoutes); err != nil {
		return err
	}

	// Proxy routes use the default gateway realm with full passthrough
	for _, routeConfig := range proxyRoutes {
		route, err := createManagedRoute(ctx, handlingContext, routeConfig, defaultGatewayRealm, true)
		if err != nil {
			return err
		}
		zone.Status.ManagedRoutes = append(zone.Status.ManagedRoutes, *types.ObjectRefFromObject(route))
	}

	return nil
}

func reconcileTeamAPIRoutes(ctx context.Context, handlingContext HandlingContext, zone *adminv1.Zone, identityProvider *identityapi.IdentityProvider, gateway *gatewayapi.Gateway, routes []adminv1.ManagedRouteConfig) error {
	if len(routes) == 0 {
		zone.Status.TeamApiIdentityRealm = nil
		zone.Status.TeamApiGatewayRealm = nil
		zone.Status.Links.TeamIssuer = ""
		return nil
	}

	teamApiIdentityRealm, err := createIdentityRealm(ctx, handlingContext, identityProvider, naming.ForTeamApiIdentityRealm(handlingContext.Environment), nil)
	if err != nil {
		return err
	}
	zone.Status.TeamApiIdentityRealm = types.ObjectRefFromObject(teamApiIdentityRealm)

	teamApisGatewayRealm, err := createGatewayRealm(ctx, handlingContext, gateway, naming.ForTeamApiGatewayRealm(handlingContext.Environment))
	if err != nil {
		return err
	}
	zone.Status.TeamApiGatewayRealm = types.ObjectRefFromObject(teamApisGatewayRealm)
	if len(teamApisGatewayRealm.Spec.IssuerUrls) > 0 {
		zone.Status.Links.TeamIssuer = teamApisGatewayRealm.Spec.IssuerUrls[0]
	}

	for _, routeConfig := range routes {
		route, err := createManagedRoute(ctx, handlingContext, routeConfig, teamApisGatewayRealm, false)
		if err != nil {
			return err
		}
		zone.Status.ManagedRoutes = append(zone.Status.ManagedRoutes, *types.ObjectRefFromObject(route))
	}

	return nil
}

func createManagedRoute(ctx context.Context, handlingContext HandlingContext, routeConfig adminv1.ManagedRouteConfig, gatewayRealm *gatewayapi.Realm, passThrough bool) (*gatewayapi.Route, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayRealm.Name + "--" + naming.ForGatewayRoute(routeConfig),
			Namespace: handlingContext.Namespace.Name,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			cconfig.EnvironmentLabelKey:          handlingContext.Environment.Name,
			cconfig.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
			cconfig.OwnerUidLabelKey:             string(handlingContext.Zone.GetUID()),
		}

		upstreamUrl, err := url.Parse(routeConfig.Url)
		if err != nil {
			return errors.Wrapf(err, "Cannot parse upstream url of internal route %s", routeConfig.Url)
		}
		upstream := gatewayapi.Upstream{
			Scheme: upstreamUrl.Scheme,
			Host:   upstreamUrl.Hostname(),
			Port:   gatewayapi.GetPortOrDefaultFromScheme(upstreamUrl),
			Path:   upstreamUrl.Path,
		}

		downstreamUrl, err := urls.ForRouteDownstream(handlingContext.Zone.Spec.Gateway.Url, routeConfig)
		if err != nil {
			return err
		}
		issuerUrl := ""
		if !passThrough && len(gatewayRealm.Spec.IssuerUrls) > 0 {
			issuerUrl = gatewayRealm.Spec.IssuerUrls[0]
		}
		downstream := gatewayapi.Downstream{
			Host:      downstreamUrl.Host,
			Port:      0,
			Path:      downstreamUrl.Path,
			IssuerUrl: issuerUrl,
		}

		route.Spec = gatewayapi.RouteSpec{
			Realm:       *types.ObjectRefFromObject(gatewayRealm),
			PassThrough: passThrough,
			Upstreams:   []gatewayapi.Upstream{upstream},
			Downstreams: []gatewayapi.Downstream{downstream},
			Traffic:     gatewayapi.Traffic{},
		}

		if !passThrough {
			route.Spec.Security = &gatewayapi.Security{
				DisableAccessControl: true,
			}
		}

		return nil
	}

	_, err := scopedClient.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Gateway route %s in zone %s", route.GetName(), handlingContext.Zone.Name)
	}
	return route, nil
}

func createGatewayConsumer(ctx context.Context, handlingContext HandlingContext, gatewayRealm *gatewayapi.Realm) (*gatewayapi.Consumer, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	gatewayConsumer := &gatewayapi.Consumer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.ForGatewayConsumer(),
			Namespace: handlingContext.Namespace.Name,
		},
	}

	mutator := func() error {
		gatewayConsumer.Labels = map[string]string{
			cconfig.EnvironmentLabelKey:          handlingContext.Environment.Name,
			cconfig.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
		}

		gatewayConsumer.Spec = gatewayapi.ConsumerSpec{
			Realm: *types.ObjectRefFromObject(gatewayRealm),
			Name:  naming.ForGatewayConsumer(),
		}
		return nil
	}

	_, err := scopedClient.CreateOrUpdate(ctx, gatewayConsumer, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Gateway Consumer %s in zone %s", naming.ForGatewayConsumer(), handlingContext.Zone.Name)
	}
	return gatewayConsumer, nil
}

func createGatewayRealm(ctx context.Context, handlingContext HandlingContext, gateway *gatewayapi.Gateway, realmName string) (*gatewayapi.Realm, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	gatewayRealm := &gatewayapi.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      realmName,
			Namespace: handlingContext.Namespace.Name,
		},
	}

	mutator := func() error {
		gatewayRealm.Labels = map[string]string{
			cconfig.EnvironmentLabelKey:          handlingContext.Environment.Name,
			cconfig.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
		}

		var routeOverwrites []gatewayapi.RouteOverwrite
		// If the zone is WORLD visible, the gateway is considered a "SpaceGate"
		// to reduce internet-facing exposure the actual IDP routes are not exposed directly
		// but via a proxy route "/auth/realms/<realm>". However, this path is already used for
		// the Gateway Realm itself, so we need to add another prefix to avoid conflicts.
		// The SpaceGate route will then be available under a common-prefix
		if handlingContext.Zone.Spec.Visibility == adminv1.ZoneVisibilityWorld {
			for _, rt := range []gatewayapi.RouteType{
				gatewayapi.RouteTypeIssuer,
				gatewayapi.RouteTypeCerts,
				gatewayapi.RouteTypeDiscovery,
			} {
				routeOverwrites = append(routeOverwrites, gatewayapi.RouteOverwrite{
					Type:       rt,
					Enabled:    true,
					PathPrefix: spacegatePathPrefix,
				})
			}
		}

		gatewayRealm.Spec = gatewayapi.RealmSpec{
			Gateway:          types.ObjectRefFromObject(gateway),
			Urls:             []string{handlingContext.Zone.Spec.Gateway.Url},
			IssuerUrls:       []string{urls.ForGatewayRealm(handlingContext.Zone.Spec.IdentityProvider.Url, realmName)},
			DefaultConsumers: []string{},
			RouteOverwrites:  routeOverwrites,
		}
		return nil
	}

	_, err := scopedClient.CreateOrUpdate(ctx, gatewayRealm, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Gateway Realm %s in zone %s", handlingContext.Environment.Name, handlingContext.Zone.Name)
	}
	return gatewayRealm, nil
}

func createGateway(ctx context.Context, handlingContext HandlingContext) (*gatewayapi.Gateway, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	gateway := &gatewayapi.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(naming.ForGateway()),
			Namespace: labelutil.NormalizeValue(handlingContext.Namespace.Name),
		},
	}

	mutator := func() error {
		gateway.Labels = map[string]string{
			cconfig.EnvironmentLabelKey:          handlingContext.Environment.Name,
			cconfig.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
		}

		var adminUrl string
		if handlingContext.Zone.Spec.Gateway.Admin.Url != nil {
			adminUrl = *handlingContext.Zone.Spec.Gateway.Admin.Url
		} else {
			adminUrl = urls.ForGatewayAdminUrl(handlingContext.Zone.Spec.Gateway.Url)
		}

		gateway.Spec = gatewayapi.GatewaySpec{
			Admin: gatewayapi.AdminConfig{
				ClientId:     naming.ForGatewayAdminClientId(),
				ClientSecret: handlingContext.Zone.Spec.Gateway.Admin.ClientSecret,
				IssuerUrl:    urls.ForGatewayAdminIssuerUrl(handlingContext.Zone.Spec.IdentityProvider.Url),
				Url:          adminUrl,
			},
			Redis: gatewayapi.RedisConfig{
				Host:      handlingContext.Zone.Spec.Redis.Host,
				Port:      handlingContext.Zone.Spec.Redis.Port,
				Password:  handlingContext.Zone.Spec.Redis.Password,
				EnableTLS: handlingContext.Zone.Spec.Redis.EnableTLS,
			},
		}

		return nil
	}
	_, err := scopedClient.CreateOrUpdate(ctx, gateway, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Gateway: %s in zone: %s", gateway.Name, handlingContext.Zone.Name)
	}
	return gateway, nil
}

func createIdentityClient(ctx context.Context, handlingContext HandlingContext, identityRealm *identityapi.Realm) (*identityapi.Client, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	identityClient := &identityapi.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(naming.ForGatewayClient()),
			Namespace: labelutil.NormalizeValue(handlingContext.Namespace.Name),
		},
	}

	mutator := func() error {
		identityClient.Labels = map[string]string{
			cconfig.EnvironmentLabelKey:          handlingContext.Environment.Name,
			cconfig.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
		}

		var clientSecret string
		// we don't want to rotate the secret everytime the zone is processed
		existingClient, err := getIdentityClient(ctx, types.ObjectRefFromObject(identityClient))
		if err != nil {
			clientSecret = uuid.NewString()
		} else {
			clientSecret = existingClient.Spec.ClientSecret
		}

		identityClient.Spec = identityapi.ClientSpec{
			Realm:    types.ObjectRefFromObject(identityRealm),
			ClientId: naming.ForGatewayClient(),
			// the value will come from a call to the secrets manager, currently stays like this
			ClientSecret: clientSecret,
		}
		return nil
	}
	_, err := scopedClient.CreateOrUpdate(ctx, identityClient, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Identity Client: %s in zone: %s", identityClient.Name, handlingContext.Zone.Name)
	}
	return identityClient, nil
}

func getIdentityClient(ctx context.Context, ref *types.ObjectRef) (*identityapi.Client, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	identityClient := &identityapi.Client{}
	err := c.Get(ctx, ref.K8s(), identityClient)
	if err != nil {
		return nil, errors.Wrapf(err, "faled to get identity client %s", ref.Name)
	}
	return identityClient, nil
}

func createIdentityRealm(ctx context.Context, handlingContext HandlingContext, identityProvider *identityapi.IdentityProvider, realmName string, claims []identityapi.ClaimConfig) (*identityapi.Realm, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	identityRealm := &identityapi.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(realmName),
			Namespace: labelutil.NormalizeValue(handlingContext.Namespace.Name),
		},
	}

	mutator := func() error {
		identityRealm.Labels = map[string]string{
			cconfig.EnvironmentLabelKey:          handlingContext.Environment.Name,
			cconfig.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
		}

		identityRealm.Spec = identityapi.RealmSpec{
			IdentityProvider: &types.ObjectRef{
				Name:      identityProvider.Name,
				Namespace: identityProvider.Namespace,
			},
			Claims: claims,
		}

		secretRotationConfig := handlingContext.Zone.Spec.IdentityProvider.SecretRotation
		if secretRotationConfig != nil && secretRotationConfig.Enabled {
			identityRealm.Spec.SecretRotation = &identityapi.SecretRotationConfig{
				GracePeriod:             secretRotationConfig.GracePeriod,
				ExpirationPeriod:        secretRotationConfig.ExpirationPeriod,
				RemainingRotationPeriod: secretRotationConfig.ExpirationPeriod, // same as expiration to allow rotation immediately after creation if needed
			}

			handlingContext.Zone.EnableFeature(adminv1.FeatureSecretRotation)
		} else {
			identityRealm.Spec.SecretRotation = nil
			handlingContext.Zone.ManageFeature(adminv1.FeatureSecretRotation, false)
		}

		return nil
	}
	_, err := scopedClient.CreateOrUpdate(ctx, identityRealm, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Identity Realm: %s in zone: %s", identityRealm.Name, handlingContext.Zone.Name)
	}
	return identityRealm, nil
}

func createIdentityProvider(ctx context.Context, handlingContext HandlingContext) (*identityapi.IdentityProvider, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	identityProvider := &identityapi.IdentityProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(naming.ForIdentityProvider(handlingContext.Zone)),
			Namespace: labelutil.NormalizeValue(handlingContext.Namespace.Name),
		},
	}

	mutator := func() error {
		identityProvider.Labels = map[string]string{
			cconfig.EnvironmentLabelKey:          handlingContext.Environment.Name,
			cconfig.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
		}

		var adminUrl string
		if handlingContext.Zone.Spec.IdentityProvider.Admin.Url != nil {
			adminUrl = *handlingContext.Zone.Spec.IdentityProvider.Admin.Url
		} else {
			adminUrl = urls.ForIdentityProviderAdminUrl(handlingContext.Zone.Spec.IdentityProvider.Url)
		}

		identityProvider.Spec = identityapi.IdentityProviderSpec{
			AdminUrl:      adminUrl,
			AdminPassword: handlingContext.Zone.Spec.IdentityProvider.Admin.Password,
			AdminClientId: handlingContext.Zone.Spec.IdentityProvider.Admin.ClientId,
			AdminUserName: handlingContext.Zone.Spec.IdentityProvider.Admin.UserName,
		}

		return nil
	}

	_, err := scopedClient.CreateOrUpdate(ctx, identityProvider, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update IdentityProvider: %s in zone: %s", identityProvider.Name, handlingContext.Zone.Name)
	}
	return identityProvider, nil
}

func (h *ZoneHandler) Delete(ctx context.Context, obj *adminv1.Zone) error {
	return nil
}
