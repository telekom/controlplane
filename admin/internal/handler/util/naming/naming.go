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
	gatewayConsumerClientId    = "gateway"
)

// ForDefaultIdentityRealm returns the default identity realm name for the given environment.
func ForDefaultIdentityRealm(environment *adminv1.Environment) string {
	return environment.GetRealmName()
}

// ForInternalIdentityRealm returns the internal identity realm name. Always "rover".
func ForInternalIdentityRealm() string {
	return internalIdentityRealmName
}

// ForTeamApiIdentityRealm returns the identity realm name for the Team API in the given environment.
// The name is prefixed with "team-".
func ForTeamApiIdentityRealm(environment *adminv1.Environment) string {
	return teamApiIdentityRealmPrefix + environment.GetRealmName()
}

// ForIdentityProvider returns the metadata.name that is used for the IdentityProvider CR
func ForIdentityProvider(zone *adminv1.Zone, identityProviderName string) string {
	return labelutil.NormalizeValue(zone.Name + "-" + identityProviderName)
}

// ForGatewayAdminClientId returns the spec.ClientID that is used for the Client CR
func ForGatewayAdminClientId() string {
	return labelutil.NormalizeValue(gatewayAdminClientId)
}

// ForGatewayAdminClient returns the metadata.name that is used for the Client CR
func ForGatewayAdminClient(idpName string) string {
	return labelutil.NormalizeValue(gatewayAdminClientId + "-" + idpName)
}

// ForGateway returns the metadata.name that is used for the Gateway CR
func ForGateway(zone *adminv1.Zone, gatewayName string) string {
	return labelutil.NormalizeValue(zone.Name + "-" + gatewayName)
}

// ForGatewayConsumer returns the metadata.name that is used for the Consumer CR
func ForGatewayConsumer(zone *adminv1.Zone, gatewayName string) string {
	return labelutil.NormalizeValue(gatewayConsumerClientId + "-" + gatewayName)
}

// ForGatewayConsumerName returns the clientID used to identify the gateway consumer.
// This always needs to be "gateway".
func ForGatewayConsumerClientID() string {
	return gatewayConsumerClientId
}

func ForGatewayRoute(config adminv1.ManagedRouteConfig) string {
	return config.Name
}
