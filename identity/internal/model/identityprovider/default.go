// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

import (
	"github.com/telekom/controlplane/common/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

func NewIdentityProviderSpec() *identityv1.IdentityProviderSpec {
	return &identityv1.IdentityProviderSpec{
		AdminUrl:      "https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/admin/realms/",
		AdminClientId: "admin-cli",
		AdminUserName: "admin",
		AdminPassword: "password",
	}
}

func NewIdentityProviderMeta(name string, namespace string, environment string) *metav1.ObjectMeta {
	return &metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Labels: map[string]string{
			config.EnvironmentLabelKey: environment,
		},
	}
}

func NewIdentityProvider(name string, namespace string, environment string) *identityv1.IdentityProvider {
	return &identityv1.IdentityProvider{
		ObjectMeta: *NewIdentityProviderMeta(name, namespace, environment),
		Spec:       *NewIdentityProviderSpec(),
	}
}
