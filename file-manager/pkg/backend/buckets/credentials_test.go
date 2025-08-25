// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockProperties struct {
	props map[string]string
}

func newMockProperties(props map[string]string) Properties {
	return &mockProperties{props: props}
}

func (m *mockProperties) GetDefault(key string, defaultValue string) string {
	if val, ok := m.props[key]; ok {
		return val
	}
	return defaultValue
}

func (m *mockProperties) Get(key string) string {
	return m.props[key]
}

func TestAutoDiscoverProvider(t *testing.T) {
	t.Run("detect STS web identity", func(t *testing.T) {
		props := newMockProperties(map[string]string{
			"roleArn": "arn:aws:iam::123456789012:role/test-role",
		})
		provider := AutoDiscoverProvider(props)
		assert.Equal(t, CredentialProviderSTSWebIdentity, provider)
	})

	t.Run("detect static credentials", func(t *testing.T) {
		props := newMockProperties(map[string]string{
			"accessKey": "test-access-key",
			"secretKey": "test-secret-key",
		})
		provider := AutoDiscoverProvider(props)
		assert.Equal(t, CredentialProviderStatic, provider)
	})

	t.Run("default to env minio", func(t *testing.T) {
		props := newMockProperties(map[string]string{})
		provider := AutoDiscoverProvider(props)
		assert.Equal(t, CredentialProviderEnvMinio, provider)
	})
}

func TestNewCredentials(t *testing.T) {
	t.Run("create static credentials", func(t *testing.T) {
		props := newMockProperties(map[string]string{
			"accessKey": "test-access-key",
			"secretKey": "test-secret-key",
		})

		creds, err := NewCredentials(CredentialProviderStatic, WithProperties(props))

		assert.NoError(t, err)
		assert.NotNil(t, creds)
	})

	t.Run("create env minio credentials", func(t *testing.T) {
		creds, err := NewCredentials(CredentialProviderEnvMinio)

		assert.NoError(t, err)
		assert.NotNil(t, creds)
	})

	t.Run("fail with invalid provider", func(t *testing.T) {
		creds, err := NewCredentials("invalid-provider")

		assert.Error(t, err)
		assert.Nil(t, creds)
		assert.Contains(t, err.Error(), "unknown credential provider type")
	})

	t.Run("fail with missing static credentials", func(t *testing.T) {
		props := newMockProperties(map[string]string{
			"accessKey": "test-access-key",
			// Missing secretKey
		})

		creds, err := NewCredentials(CredentialProviderStatic, WithProperties(props))

		assert.Error(t, err)
		assert.Nil(t, creds)
		assert.Contains(t, err.Error(), "require both accessKey and secretKey")
	})
}

func TestCredentialOptions(t *testing.T) {
	t.Run("WithProperties", func(t *testing.T) {
		props := newMockProperties(map[string]string{"key": "value"})
		options := &CredentialOptions{}
		WithProperties(props)(options)
		assert.Equal(t, props, options.properties)
	})

	t.Run("WithProperty", func(t *testing.T) {
		options := &CredentialOptions{}
		WithProperty("key", "value")(options)
		assert.NotNil(t, options.properties)
		assert.Equal(t, "value", options.properties.Get("key"))
	})
}
