// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/assert"
)

func TestNewBucketConfig(t *testing.T) {
	t.Run("successful config creation", func(t *testing.T) {
		// Mock credentials that always return a value
		creds := credentials.NewStaticV4("test-access", "test-secret", "")

		config, err := NewBucketConfig(
			WithEndpoint("localhost:9000"),
			WithBucketName("test-bucket"),
			WithCredentials(creds),
			WithLogger(logr.Discard()),
		)

		assert.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "localhost:9000", config.Endpoint)
		assert.Equal(t, "test-bucket", config.BucketName)
		assert.Equal(t, creds, config.Credentials)
		assert.NotNil(t, config.Client)
	})

	t.Run("fails without credentials", func(t *testing.T) {
		config, err := NewBucketConfig(
			WithEndpoint("localhost:9000"),
			WithBucketName("test-bucket"),
		)

		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "credentials are not set")
	})
}

func TestConfigOptions(t *testing.T) {
	t.Run("WithEndpoint", func(t *testing.T) {
		config := &BucketConfig{}
		WithEndpoint("test-endpoint")(config)
		assert.Equal(t, "test-endpoint", config.Endpoint)
	})

	t.Run("WithBucketName", func(t *testing.T) {
		config := &BucketConfig{}
		WithBucketName("test-bucket")(config)
		assert.Equal(t, "test-bucket", config.BucketName)
	})

	t.Run("WithLogger", func(t *testing.T) {
		config := &BucketConfig{Logger: logr.Discard()}
		logger := logr.Discard().WithName("test-logger")
		WithLogger(logger)(config)
		assert.Equal(t, logger, config.Logger)
	})

	t.Run("WithCredentials", func(t *testing.T) {
		config := &BucketConfig{}
		creds := credentials.NewStaticV4("test-access", "test-secret", "")
		WithCredentials(creds)(config)
		assert.Equal(t, creds, config.Credentials)
	})

	t.Run("WithInsecureSkipTLS true", func(t *testing.T) {
		config := &BucketConfig{}
		WithInsecureSkipTLS(true)(config)
		assert.True(t, config.InsecureSkipTLS)
	})

	t.Run("WithInsecureSkipTLS false", func(t *testing.T) {
		config := &BucketConfig{}
		WithInsecureSkipTLS(false)(config)
		assert.False(t, config.InsecureSkipTLS)
	})
}

func TestInsecureSkipTLS_ClientCreation(t *testing.T) {
	creds := credentials.NewStaticV4("test-access", "test-secret", "")

	t.Run("default is secure (TLS enabled)", func(t *testing.T) {
		config, err := NewBucketConfig(
			WithEndpoint("localhost:9000"),
			WithBucketName("test-bucket"),
			WithCredentials(creds),
		)
		assert.NoError(t, err)
		assert.False(t, config.InsecureSkipTLS)
		assert.NotNil(t, config.Client)
	})

	t.Run("insecure skip TLS creates client successfully", func(t *testing.T) {
		config, err := NewBucketConfig(
			WithEndpoint("localhost:9000"),
			WithBucketName("test-bucket"),
			WithCredentials(creds),
			WithInsecureSkipTLS(true),
		)
		assert.NoError(t, err)
		assert.True(t, config.InsecureSkipTLS)
		assert.NotNil(t, config.Client)
	})
}
