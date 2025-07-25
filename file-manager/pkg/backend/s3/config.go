// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
)

// S3Config holds all configuration needed for S3 operations
type S3Config struct {
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

// NewS3Config creates a new S3 configuration with the provided options
func NewS3Config(options ...ConfigOption) (*S3Config, error) {
	// Default configuration
	config := &S3Config{
		Logger:         logr.Discard(),
		Endpoint:       "s3.amazonaws.com",
		STSEndpoint:    "https://sts.amazonaws.com",
		BucketName:     "s3-ec1-d-distcp-tardis",
		RoleSessionArn: "arn:aws:iam::540220622237:role/s3-ec1-d-distcp-tardis_Role",
		TokenPath:      "/var/run/secrets/tokens/sa-token",
	}

	// Apply all options
	for _, option := range options {
		option(config)
	}

	// Initialize the S3 client
	client, err := config.initClient()
	if err != nil {
		config.Logger.Error(err, "Failed to initialize S3 client")
		return nil, err
	}

	config.Client = client
	return config, nil
}

// NewS3ConfigWithLogger creates a new S3 configuration with the provided logger and options
func NewS3ConfigWithLogger(log logr.Logger, options ...ConfigOption) (*S3Config, error) {
	// Add logger to options if we got a real logger (not Discard)
	if log != logr.Discard() {
		options = append(options, WithLogger(log))
	}

	return NewS3Config(options...)
}

// createMinioClient creates a new Minio S3 client with the given credentials
func (c *S3Config) createMinioClient(creds *credentials.Credentials) (*minio.Client, error) {
	c.Logger.V(1).Info("Creating Minio S3 client", "endpoint", c.Endpoint)
	client, err := minio.New(c.Endpoint, &minio.Options{
		Creds:  creds,
		Secure: true,
	})
	if err != nil {
		c.Logger.Error(err, "Failed to create S3 client")
		return nil, errors.Wrap(err, "failed to create S3 client")
	}
	return client, nil
}

// ConfigOption is a function type for applying options to S3Config
type ConfigOption func(*S3Config)

// WithEndpoint sets the S3 endpoint
func WithEndpoint(endpoint string) ConfigOption {
	return func(c *S3Config) {
		c.Endpoint = endpoint
	}
}

// WithSTSEndpoint sets the STS endpoint
func WithSTSEndpoint(stsEndpoint string) ConfigOption {
	return func(c *S3Config) {
		c.STSEndpoint = stsEndpoint
	}
}

// WithBucketName sets the bucket name
func WithBucketName(bucketName string) ConfigOption {
	return func(c *S3Config) {
		c.BucketName = bucketName
	}
}

// WithRoleSessionArn sets the role session ARN
func WithRoleSessionArn(roleArn string) ConfigOption {
	return func(c *S3Config) {
		c.RoleSessionArn = roleArn
	}
}

// WithTokenPath sets the path to the token file
func WithTokenPath(path string) ConfigOption {
	return func(c *S3Config) {
		c.TokenPath = path
	}
}

// WithLogger sets the logger
func WithLogger(logger logr.Logger) ConfigOption {
	return func(c *S3Config) {
		c.Logger = logger
	}
}

// initClient initializes the Minio client with the current configuration
func (c *S3Config) initClient() (*minio.Client, error) {
	log := c.Logger

	log.V(1).Info("Initializing S3 client",
		"endpoint", c.Endpoint,
		"stsEndpoint", c.STSEndpoint,
		"bucketName", c.BucketName,
		"roleARN", c.RoleSessionArn)

	// Get initial token for client creation
	token, tokenAvailable, _ := c.getTokenFromSources()

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

	// Create S3 client
	client, err := c.createMinioClient(creds)
	if err != nil {
		return nil, err
	}

	log.V(1).Info("S3 client created successfully")
	return client, nil
}
