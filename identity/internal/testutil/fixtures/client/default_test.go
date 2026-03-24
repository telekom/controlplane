// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/telekom/controlplane/common/pkg/config"
)

const (
	name         = "test-name"
	namespace    = "test-namespace"
	environment  = "test-environment"
	clientId     = "test-client"
	clientSecret = "test-secret"
)

func TestNewClientSpecIsCreatedCorrectly(t *testing.T) {
	spec := NewClientSpec(name, namespace)

	assert.NotNil(t, spec)
	assert.Equal(t, name, spec.Realm.Name)
	assert.Equal(t, namespace, spec.Realm.Namespace)
	assert.Equal(t, clientId, spec.ClientId)
	assert.Equal(t, clientSecret, spec.ClientSecret)
}

func TestNewClientMetaIsCreatedCorrectly(t *testing.T) {
	meta := NewClientMeta(name, namespace, environment)

	assert.NotNil(t, meta)
	assert.Equal(t, name, meta.Name)
	assert.Equal(t, namespace, meta.Namespace)
	assert.Equal(t, environment, meta.Labels[config.EnvironmentLabelKey])
}

func TestNewClientIsCreatedCorrectly(t *testing.T) {
	realmName := "test-realm"

	client := NewClient(name, namespace, environment, realmName)

	assert.NotNil(t, client)
	assert.Equal(t, name, client.ObjectMeta.Name)
	assert.Equal(t, namespace, client.ObjectMeta.Namespace)
	assert.Equal(t, environment, client.ObjectMeta.Labels[config.EnvironmentLabelKey])
	assert.Equal(t, realmName, client.Spec.Realm.Name)
	assert.Equal(t, namespace, client.Spec.Realm.Namespace)
	assert.Equal(t, clientId, client.Spec.ClientId)
	assert.Equal(t, clientSecret, client.Spec.ClientSecret)
}
