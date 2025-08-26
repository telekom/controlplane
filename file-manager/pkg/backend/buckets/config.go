// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
)

const (
	// BucketsBackendTokenPath per convention - /var/run/secrets/:backendName/token
	BucketsBackendTokenPath = "/var/run/secrets/buckets/token"
)

// BucketConfig holds all configuration needed for bucket operations
type BucketConfig struct {
	Endpoint    string
	BucketName  string
	Client      *minio.Client
	Logger      logr.Logger
	Credentials *credentials.Credentials
}

// NewBucketConfig creates a new bucket configuration with the provided options
func NewBucketConfig(options ...ConfigOption) (*BucketConfig, error) {
	// Default configuration
	config := &BucketConfig{
		Logger: logr.Discard(),
	}

	// Apply all options
	for _, option := range options {
		option(config)
	}

	// Initialize the bucket client
	client, err := config.initClient()
	if err != nil {
		config.Logger.Error(err, "Failed to initialize bucket client")
		return nil, err
	}

	config.Client = client
	return config, nil
}

// ConfigOption is a function type for applying options to BucketConfig
type ConfigOption func(*BucketConfig)

func WithCredentials(creds *credentials.Credentials) ConfigOption {
	return func(c *BucketConfig) {
		c.Credentials = creds
	}
}

// WithEndpoint sets the bucket endpoint
func WithEndpoint(endpoint string) ConfigOption {
	return func(c *BucketConfig) {
		c.Endpoint = endpoint
	}
}

// WithBucketName sets the bucket name
func WithBucketName(bucketName string) ConfigOption {
	return func(c *BucketConfig) {
		c.BucketName = bucketName
	}
}

// WithLogger sets the logger
func WithLogger(logger logr.Logger) ConfigOption {
	return func(c *BucketConfig) {
		c.Logger = logger
	}
}

// initClient initializes the Minio client with the current configuration
func (c *BucketConfig) initClient() (*minio.Client, error) {
	c.Logger.V(1).Info("Creating Minio client", "endpoint", c.Endpoint)

	if c.Credentials == nil {
		return nil, errors.New("credentials are not set")
	}

	client, err := minio.New(c.Endpoint, &minio.Options{
		Creds:           c.Credentials,
		Secure:          true,
		TrailingHeaders: true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create client")
	}
	return client, nil
}
