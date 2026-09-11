// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

func NewClientSpec(realmName, namespace string) *identityv1.ClientSpec {
	return &identityv1.ClientSpec{
		Realm: &types.ObjectRef{
			Name:      realmName,
			Namespace: namespace,
		},
		ClientId:     "test-client",
		ClientSecret: "test-secret",
	}
}

func NewClientMeta(name, namespace, environment string) *metav1.ObjectMeta {
	return &metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Labels: map[string]string{
			config.EnvironmentLabelKey: environment,
		},
	}
}

func NewClient(name, namespace, environment, realmName string) *identityv1.Client {
	return &identityv1.Client{
		ObjectMeta: *NewClientMeta(name, namespace, environment),
		Spec:       *NewClientSpec(realmName, namespace),
	}
}
