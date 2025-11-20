// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

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

func TestIdentityProviderSpecIsCreatedCorrectly(t *testing.T) {
	spec := NewIdentityProviderSpec()

	assert.NotNil(t, spec)
	assert.Equal(t, "https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/admin/realms/", spec.AdminUrl)
	assert.Equal(t, "admin-cli", spec.AdminClientId)
	assert.Equal(t, "admin", spec.AdminUserName)
	assert.Equal(t, "password", spec.AdminPassword)
}

func TestIdentityProviderMetaIsCreatedCorrectly(t *testing.T) {
	meta := NewIdentityProviderMeta(name, namespace, environment)

	assert.NotNil(t, meta)
	assert.Equal(t, name, meta.Name)
	assert.Equal(t, namespace, meta.Namespace)
	assert.Equal(t, environment, meta.Labels[config.EnvironmentLabelKey])
}

func TestIdentityProviderIsCreatedCorrectly(t *testing.T) {
	provider := NewIdentityProvider(name, namespace, environment)

	assert.NotNil(t, provider)
	assert.Equal(t, name, provider.Name)
	assert.Equal(t, namespace, provider.Namespace)
	assert.Equal(t, environment, provider.Labels[config.EnvironmentLabelKey])
	assert.Equal(t, "https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/admin/realms/", provider.Spec.AdminUrl)
	assert.Equal(t, "admin-cli", provider.Spec.AdminClientId)
	assert.Equal(t, "admin", provider.Spec.AdminUserName)
	assert.Equal(t, "password", provider.Spec.AdminPassword)
}
