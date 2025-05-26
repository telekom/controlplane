// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package naming

import adminv1 "github.com/telekom/controlplane/admin/api/v1"

const (
	teamApiIdentityRealmPrefix = "team-"
	gatewayClientName          = "gateway"
	gatewayAdminClientId       = "rover"
	gateway                    = "gateway"
	gatewayConsumer            = "gateway"
)

func ForDefaultIdentityRealm(environment *adminv1.Environment) string {
	return environment.GetName()
}

func ForTeamApiIdentityRealm(environment *adminv1.Environment) string {
	return teamApiIdentityRealmPrefix + environment.GetName()
}

func ForDefaultGatewayRealm(environment *adminv1.Environment) string {
	return environment.GetName()
}

func ForTeamApiGatewayRealm(environment *adminv1.Environment) string {
	return teamApiIdentityRealmPrefix + environment.GetName()
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

func ForGatewayRoute(config adminv1.ApiConfig) string {
	return config.Name
}
