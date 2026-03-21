// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func createRover(name string, ns string, env string, spec roverv1.RoverSpec) *roverv1.Rover {
	rover := &roverv1.Rover{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				config.EnvironmentLabelKey: env,
			},
		},
		Spec: spec,
	}

	return rover
}

func newTeam(name, group, namespace, env string) *organizationv1.Team {
	return &organizationv1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      group + "--" + name,
			Namespace: namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: env,
			},
		},
		Spec: organizationv1.TeamSpec{
			Name:     name,
			Group:    group,
			Email:    "team@mail.de",
			Category: organizationv1.TeamCategoryCustomer,
			Members:  []organizationv1.Member{{Email: "member@mail.de", Name: "member"}},
		},
		Status: organizationv1.TeamStatus{},
	}
}

func newZoneWithDtc(name, namespace, dtcUrl, idpUrl string, visibility adminv1.ZoneVisibility) *adminv1.Zone {
	gatewayAdminUrl := "https://test-stargate.de/admin-api"
	idpAdminUrl := "https://test-iris.de/auth/admin/realms"

	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: adminv1.ZoneSpec{
			IdentityProvider: adminv1.IdentityProviderConfig{
				Admin: adminv1.IdentityProviderAdminConfig{
					Url:      &idpAdminUrl,
					ClientId: "test-idp-admin-id",
					UserName: "test-idp-admin-username",
					Password: "test-idp-admin-password",
				},
				Url: idpUrl,
			},
			Gateway: adminv1.GatewayConfig{
				Admin: adminv1.GatewayAdminConfig{
					ClientSecret: "test-gateway-admin-secret",
					Url:          &gatewayAdminUrl,
				},
				Url:    "https://test-stargate-" + name + ".de/",
				DtcUrl: &dtcUrl,
			},
			Redis: adminv1.RedisConfig{
				Host:      "http://test-redis.de/",
				Port:      123,
				Password:  "test-redis-password",
				EnableTLS: true,
			},
			Visibility: visibility,
		},
	}
}
