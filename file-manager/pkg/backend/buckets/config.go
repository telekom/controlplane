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

// BucketConfig holds all configuration needed for bucket operations
type BucketConfig struct {
	Endpoint       string
	STSEndpoint    string
	BucketName     string
	RoleSessionArn string
	TokenPath      string
	Client         *minio.Client
	Logger         logr.Logger
	// Current bearer token and credentials
	currentToken string
	currentCreds *credentials.Credentials
}

// NewBucketConfig creates a new bucket configuration with the provided options
func NewBucketConfig(options ...ConfigOption) (*BucketConfig, error) {
	// Default configuration
	config := &BucketConfig{
		Logger:         logr.Discard(),
		Endpoint:       "s3.amazonaws.com",
		STSEndpoint:    "https://sts.amazonaws.com",
		BucketName:     "my-s3-bucket",
		RoleSessionArn: "arn:aws:iam::123456789012:role/my-sample-role",
		TokenPath:      "/var/run/files/filemgr/filemgr-token",
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

// NewBucketConfigWithLogger creates a new bucket configuration with the provided logger and options
func NewBucketConfigWithLogger(log logr.Logger, options ...ConfigOption) (*BucketConfig, error) {
	// Add logger to options if we got a real logger (not Discard)
	if log != logr.Discard() {
		options = append(options, WithLogger(log))
	}

	return NewBucketConfig(options...)
}

// createMinioClient creates a new Minio client with the given credentials
func (c *BucketConfig) createMinioClient(creds *credentials.Credentials) (*minio.Client, error) {
	c.Logger.V(1).Info("Creating Minio client", "endpoint", c.Endpoint)
	client, err := minio.New(c.Endpoint, &minio.Options{
		Creds:           creds,
		Secure:          true,
		TrailingHeaders: true, // Enable trailing headers for CRC64NVME checksum support
	})
	if err != nil {
		c.Logger.Error(err, "Failed to create client")
		return nil, errors.Wrap(err, "failed to create client")
	}
	return client, nil
}

// ConfigOption is a function type for applying options to BucketConfig
type ConfigOption func(*BucketConfig)

// WithEndpoint sets the bucket endpoint
func WithEndpoint(endpoint string) ConfigOption {
	return func(c *BucketConfig) {
		c.Endpoint = endpoint
	}
}

// WithSTSEndpoint sets the STS endpoint
func WithSTSEndpoint(stsEndpoint string) ConfigOption {
	return func(c *BucketConfig) {
		c.STSEndpoint = stsEndpoint
	}
}

// WithBucketName sets the bucket name
func WithBucketName(bucketName string) ConfigOption {
	return func(c *BucketConfig) {
		c.BucketName = bucketName
	}
}

// WithRoleSessionArn sets the role session ARN
func WithRoleSessionArn(roleArn string) ConfigOption {
	return func(c *BucketConfig) {
		c.RoleSessionArn = roleArn
	}
}

// WithTokenPath sets the path to the token file
func WithTokenPath(path string) ConfigOption {
	return func(c *BucketConfig) {
		c.TokenPath = path
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
	log := c.Logger

	log.V(1).Info("Initializing client",
		"endpoint", c.Endpoint,
		"stsEndpoint", c.STSEndpoint,
		"bucketName", c.BucketName,
		"roleARN", c.RoleSessionArn)

	// Get initial token for client creation
	token, tokenAvailable := c.getTokenFromSources()

	// Store the initial token if available
	if tokenAvailable {
		c.currentToken = token
	}

	// Get credentials based on token availability
	creds, err := c.getCredentials(tokenAvailable, token)
	if err != nil {
		return nil, err
	}

	// Store the credentials
	c.currentCreds = creds

	// Create client
	client, err := c.createMinioClient(creds)
	if err != nil {
		return nil, err
	}

	log.V(1).Info("S3 client created successfully")
	return client, nil
}
