// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/telekom/controlplane/common/pkg/config"
)

const (
	name        = "test-name"
	namespace   = "test-namespace"
	environment = "test-environment"
)

func TestRealmSpecIsCreatedCorrectly(t *testing.T) {
	identityProviderName := "test-identity-provider"

	spec := NewRealmSpec(identityProviderName, namespace)

	assert.NotNil(t, spec)
	assert.Equal(t, identityProviderName, spec.IdentityProvider.Name)
	assert.Equal(t, namespace, spec.IdentityProvider.Namespace)
}

func TestRealmMetaIsCreatedCorrectly(t *testing.T) {

	meta := NewRealmMeta(name, namespace, environment)

	assert.NotNil(t, meta)
	assert.Equal(t, name, meta.Name)
	assert.Equal(t, namespace, meta.Namespace)
	assert.Equal(t, environment, meta.Labels[config.EnvironmentLabelKey])
}

func TestRealmIsCreatedCorrectly(t *testing.T) {
	identityProviderName := "test-identity-provider"

	realm := NewRealm(name, namespace, environment, identityProviderName)

	assert.NotNil(t, realm)
	assert.Equal(t, name, realm.ObjectMeta.Name)
	assert.Equal(t, namespace, realm.ObjectMeta.Namespace)
	assert.Equal(t, environment, realm.ObjectMeta.Labels[config.EnvironmentLabelKey])
	assert.Equal(t, identityProviderName, realm.Spec.IdentityProvider.Name)
	assert.Equal(t, namespace, realm.Spec.IdentityProvider.Namespace)
}
