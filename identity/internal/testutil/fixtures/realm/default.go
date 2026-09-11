// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

func NewRealmSpec(identityProviderName, namespace string) *identityv1.RealmSpec {
	return &identityv1.RealmSpec{
		IdentityProvider: &types.ObjectRef{
			Name:      identityProviderName,
			Namespace: namespace,
		},
	}
}

func NewRealmMeta(name, namespace, environment string) *metav1.ObjectMeta {
	return &metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Labels: map[string]string{
			config.EnvironmentLabelKey: environment,
		},
	}
}

func NewRealm(name, namespace, environment, identityProviderName string) *identityv1.Realm {
	return &identityv1.Realm{
		ObjectMeta: *NewRealmMeta(name, namespace, environment),
		Spec:       *NewRealmSpec(identityProviderName, namespace),
	}
}
