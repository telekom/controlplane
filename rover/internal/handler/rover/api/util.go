// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"
	rover "github.com/telekom/controlplane/rover/api/v1"
)

func MakeName(ownerName, basePath, organization string) string {
	name := ownerName + "--" + strings.Trim(strings.ReplaceAll(basePath, "/", "-"), "-")
	if organization != "" {
		name = organization + "--" + name
	}
	return name
}

func toApiClient(client *rover.OAuth2ClientCredentials) *apiapi.OAuth2ClientCredentials {
	if client == nil {
		return nil
	}
	return &apiapi.OAuth2ClientCredentials{
		ClientId:     client.ClientId,
		ClientSecret: client.ClientSecret,
		ClientKey:    client.ClientKey,
		RefreshToken: client.RefreshToken,
	}
}

func toApiBasic(basic *rover.BasicAuthCredentials) *apiapi.BasicAuthCredentials {
	if basic == nil {
		return nil
	}
	return &apiapi.BasicAuthCredentials{
		Username: basic.Username,
		Password: basic.Password,
	}
}

// mapProviderFailoverZones maps the failover configuration to their corresponding list of ObjectRefs
// Its returns true if the failover configuration is not nil and has at least one zone, otherwise it returns false
func mapProviderFailoverZones(env string, failoverCfg *rover.ProviderFailover) ([]types.TypedObjectRef, bool) {
	if failoverCfg == nil || len(failoverCfg.Zones) == 0 {
		return nil, false
	}

	failoverZones := make([]types.TypedObjectRef, len(failoverCfg.Zones))
	for i, zone := range failoverCfg.Zones {
		failoverZones[i] = types.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{Kind: "Zone", APIVersion: adminv1.GroupVersion.String()},
			ObjectRef: types.ObjectRef{
				Name:      zone,
				Namespace: env,
			},
		}
	}
	return failoverZones, true
}
