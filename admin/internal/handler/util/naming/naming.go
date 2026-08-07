// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package naming

import (
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

const (
	teamApiIdentityRealmPrefix = "team-"
	internalIdentityRealmName  = "rover"
	gatewayAdminClientId       = "rover"
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

func ForIdentityProvider(zone *adminv1.Zone, identityProviderName string) string {
	return labelutil.NormalizeValue(zone.Name + "-" + identityProviderName)
}

func ForGatewayAdminClientId(gatewayName string) string {
	return labelutil.NormalizeValue(gatewayAdminClientId + "-" + gatewayName)
}

func ForGateway(zone *adminv1.Zone, gatewayName string) string {
	return labelutil.NormalizeValue(zone.Name + "-" + gatewayName)
}

func ForGatewayConsumer(zone *adminv1.Zone, gatewayName string) string {
	return labelutil.NormalizeValue(zone.Name + "-" + gatewayName)
}

func ForGatewayRoute(config adminv1.ManagedRouteConfig) string {
	return config.Name
}
