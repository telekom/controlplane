// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package naming

import adminv1 "github.com/telekom/controlplane/admin/api/v1"

const (
	teamApiIdentityRealmPrefix = "team-"
	internalIdentityRealmName  = "rover"
	gatewayClientName          = "gateway"
	gatewayAdminClientId       = "rover"
	gateway                    = "gateway"
	gatewayConsumer            = "gateway"
)

func ForDefaultIdentityRealm(environment *adminv1.Environment) string {
	return environment.GetRealmName()
}

func ForInternalIdentityRealm() string {
	return internalIdentityRealmName
}

func ForTeamApiIdentityRealm(environment *adminv1.Environment) string {
	return teamApiIdentityRealmPrefix + environment.GetRealmName()
}

func ForDefaultGatewayRealm(environment *adminv1.Environment) string {
	return environment.GetRealmName()
}

func ForTeamApiGatewayRealm(environment *adminv1.Environment) string {
	return teamApiIdentityRealmPrefix + environment.GetRealmName()
}

func ForIdentityProvider(zone *adminv1.Zone) string {
	return zone.Name
}

func ForGatewayClient() string {
	return gatewayClientName
}

func ForGatewayAdminClientId() string {
	return gatewayAdminClientId
}

func ForGateway() string {
	return gateway
}

func ForGatewayConsumer() string {
	return gatewayConsumer
}

func ForGatewayRoute(config adminv1.ManagedRouteConfig) string {
	return config.Name
}
