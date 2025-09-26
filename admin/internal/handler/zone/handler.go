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
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	"github.com/telekom/controlplane/admin/internal/handler/util/urls"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"

	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

const (
	zoneLabelName = "zone"
)

var _ handler.Handler[*adminv1.Zone] = &ZoneHandler{}

type ZoneHandler struct{}

func (h *ZoneHandler) CreateOrUpdate(ctx context.Context, obj *adminv1.Zone) error {
	envName := contextutil.EnvFromContextOrDie(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	environment := &adminv1.Environment{}
	err := c.Get(ctx, client.ObjectKey{Name: envName, Namespace: envName}, environment)
	if err != nil {
		return errors.Wrapf(err, "❌ failed to get environment %s", envName)
	}

	// Namespace
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: strings.ToLower(fmt.Sprintf("%s--%s", environment.Name, obj.Name)),
		},
	}

	mutator := func() error {
		namespace.Labels = map[string]string{
			config.EnvironmentLabelKey:          environment.Name,
			config.BuildLabelKey(zoneLabelName): obj.Name,
		}
		return nil
	}
	_, err = c.CreateOrUpdate(ctx, namespace, mutator)
	if err != nil {
		return errors.Wrapf(err, "❌ failed to create or update namespace %s", namespace.Name)
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
	identityRealm, err := createIdentityRealm(ctx, handlingContext, identityProvider, naming.ForDefaultIdentityRealm(handlingContext.Environment))
	if err != nil {
		return err
	}
	obj.Status.IdentityRealm = types.ObjectRefFromObject(identityRealm)
	obj.Status.Links.Issuer, err = url.JoinPath(obj.Spec.IdentityProvider.Url, "auth/realms/", identityRealm.Name)
	if err != nil {
		return errors.Wrapf(err, "Cannot combine identityProviderBaseUrl %s with realm name %s", obj.Spec.IdentityProvider.Url, identityRealm.Name)
	}

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

	// Team apis configuration
	if obj.Spec.TeamApis != nil {
		// Team apis identity realm
		teamApiIdentityRealm, err := createIdentityRealm(ctx, handlingContext, identityProvider, naming.ForTeamApiIdentityRealm(handlingContext.Environment))
		if err != nil {
			return err
		}
		obj.Status.TeamApiIdentityRealm = types.ObjectRefFromObject(teamApiIdentityRealm)

		// Team apis gateway realm
		teamApisGatewayRealm, err := createGatewayRealm(ctx, handlingContext, gateway, naming.ForTeamApiGatewayRealm(handlingContext.Environment))
		if err != nil {
			return err
		}
		obj.Status.TeamApiGatewayRealm = types.ObjectRefFromObject(teamApisGatewayRealm)
		obj.Status.Links.TeamIssuer = teamApisGatewayRealm.Spec.IssuerUrl

		// Team api routes
		var teamApiRouteRefs []types.ObjectRef
		for _, teamApiRoute := range obj.Spec.TeamApis.Apis {
			route, err := createTeamApiRoute(ctx, handlingContext, teamApiRoute, *teamApisGatewayRealm)
			if err != nil {
				return err
			}
			teamApiRouteRefs = append(teamApiRouteRefs, *types.ObjectRefFromObject(route))
		}
		obj.Status.TeamApiRoutes = teamApiRouteRefs
	} else {
		obj.Status.TeamApiIdentityRealm = nil
		obj.Status.TeamApiGatewayRealm = nil
		obj.Status.TeamApiRoutes = nil
		obj.Status.Links.TeamIssuer = ""
	}

	obj.SetCondition(condition.NewReadyCondition("ZoneProvisioned", "Zone has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Zone has been provisioned"))

	return nil
}

func createTeamApiRoute(ctx context.Context, handlingContext HandlingContext, teamRouteConfig adminv1.ApiConfig, gatewayRealm gatewayapi.Realm) (*gatewayapi.Route, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	teamRoute := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayRealm.Name + "--" + naming.ForGatewayRoute(teamRouteConfig),
			Namespace: handlingContext.Namespace.Name,
		},
	}

	mutator := func() error {
		teamRoute.Labels = map[string]string{
			config.EnvironmentLabelKey:          handlingContext.Environment.Name,
			config.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
		}

		upstreamUrl, err := url.Parse(teamRouteConfig.Url)
		if err != nil {
			return errors.Wrapf(err, "Cannot parse upstream url of team route %s", teamRouteConfig.Url)
		}
		upstream := gatewayapi.Upstream{
			Scheme: upstreamUrl.Scheme,
			Host:   upstreamUrl.Host,
			Port:   gatewayapi.GetPortOrDefaultFromScheme(upstreamUrl),
			Path:   upstreamUrl.Path,
		}

		downstreamUrl, err := urls.ForRouteDownstream(handlingContext.Zone.Spec.Gateway.Url, teamRouteConfig)
		if err != nil {
			return err
		}
		downstream := gatewayapi.Downstream{
			Host:      downstreamUrl.Host,
			Port:      0,
			Path:      downstreamUrl.Path,
			IssuerUrl: gatewayRealm.Spec.IssuerUrl,
		}

		teamRoute.Spec = gatewayapi.RouteSpec{
			Realm:       *types.ObjectRefFromObject(&gatewayRealm),
			PassThrough: false,
			Upstreams:   []gatewayapi.Upstream{upstream},
			Downstreams: []gatewayapi.Downstream{downstream},
			Traffic:     gatewayapi.Traffic{},
			Security: &gatewayapi.Security{
				DisableAccessControl: true, // Team APIs are not protected by ACLs
			},
		}

		return nil
	}

	_, err := scopedClient.CreateOrUpdate(ctx, teamRoute, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Gateway route %s in zone %s", teamRoute.GetName(), handlingContext.Zone.Name)
	}
	return teamRoute, nil
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
			config.EnvironmentLabelKey:          handlingContext.Environment.Name,
			config.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
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
			config.EnvironmentLabelKey:          handlingContext.Environment.Name,
			config.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
		}

		gatewayRealm.Spec = gatewayapi.RealmSpec{
			Gateway:          types.ObjectRefFromObject(gateway),
			Url:              handlingContext.Zone.Spec.Gateway.Url,
			IssuerUrl:        urls.ForGatewayRealm(handlingContext.Zone.Spec.IdentityProvider.Url, realmName),
			DefaultConsumers: []string{"gateway"},
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
			config.EnvironmentLabelKey:          handlingContext.Environment.Name,
			config.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
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
			config.EnvironmentLabelKey:          handlingContext.Environment.Name,
			config.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
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

func createIdentityRealm(ctx context.Context, handlingContext HandlingContext, identityProvider *identityapi.IdentityProvider, realmName string) (*identityapi.Realm, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	identityRealm := &identityapi.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(realmName),
			Namespace: labelutil.NormalizeValue(handlingContext.Namespace.Name),
		},
	}

	mutator := func() error {
		identityRealm.Labels = map[string]string{
			config.EnvironmentLabelKey:          handlingContext.Environment.Name,
			config.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
		}

		identityRealm.Spec = identityapi.RealmSpec{
			IdentityProvider: &types.ObjectRef{
				Name:      identityProvider.Name,
				Namespace: identityProvider.Namespace,
			},
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
			config.EnvironmentLabelKey:          handlingContext.Environment.Name,
			config.BuildLabelKey(zoneLabelName): handlingContext.Zone.Name,
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
