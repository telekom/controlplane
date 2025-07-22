// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
	"os"
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
	// Use the configured logger
	log := c.Logger

	log.V(1).Info("Initializing S3 client",
		"endpoint", c.Endpoint,
		"stsEndpoint", c.STSEndpoint,
		"bucketName", c.BucketName,
		"roleARN", c.RoleSessionArn)

	// Define the token provider function
	tokenProvider := func() (*credentials.WebIdentityToken, error) {
		if os.Getenv("MC_WEB_IDENTITY_TOKEN") != "" {
			log.V(1).Info("Using web identity token from environment variable")
			return c.getWebIDTokenFromEnv()
		}
		log.V(1).Info("Using web identity token from file", "path", c.TokenPath)
		return c.getWebIDTokenFromFile()
	}

	// Create credentials
	log.V(1).Info("Creating STS web identity credentials")
	creds, err := credentials.NewSTSWebIdentity(
		c.STSEndpoint,
		tokenProvider,
		func(i *credentials.STSWebIdentity) {
			i.RoleARN = c.RoleSessionArn
			log.V(1).Info("Setting role ARN for STS", "roleARN", i.RoleARN)
		},
	)
	if err != nil {
		log.Error(err, "Failed to create STS web identity credentials")
		return nil, errors.Wrap(err, "failed to create STS web identity credentials")
	}

	// Create S3 client
	log.V(1).Info("Creating Minio S3 client", "endpoint", c.Endpoint)
	client, err := minio.New(c.Endpoint, &minio.Options{
		Creds:           creds,
		Secure:          true,
		TrailingHeaders: true, // Enable trailing headers for checksum support
	})
	if err != nil {
		log.Error(err, "Failed to create S3 client")
		return nil, errors.Wrap(err, "failed to create S3 client")
	}
	log.V(1).Info("S3 client created successfully")

	return client, nil
}

// getWebIDTokenFromEnv retrieves the web identity token from an environment variable
func (c *S3Config) getWebIDTokenFromEnv() (*credentials.WebIdentityToken, error) {
	// Use the configured logger
	log := c.Logger

	log.V(1).Info("Retrieving web identity token from environment variable")
	data := os.Getenv("MC_WEB_IDENTITY_TOKEN")
	if data == "" {
		log.Error(nil, "MC_WEB_IDENTITY_TOKEN environment variable not set")
		return nil, errors.New("MC_WEB_IDENTITY_TOKEN environment variable not set")
	}

	log.V(1).Info("Successfully retrieved token from environment variable")
	return &credentials.WebIdentityToken{
		Token: data,
	}, nil
}

// getWebIDTokenFromFile retrieves the web identity token from a file
func (c *S3Config) getWebIDTokenFromFile() (*credentials.WebIdentityToken, error) {
	// Use the configured logger
	log := c.Logger

	log.V(1).Info("Reading web identity token from file", "path", c.TokenPath)

	// Check if file exists before trying to read it
	if _, err := os.Stat(c.TokenPath); os.IsNotExist(err) {
		log.Error(err, "Token file does not exist", "path", c.TokenPath)
		return nil, errors.New("token file does not exist")
	}

	data, err := os.ReadFile(c.TokenPath)
	if err != nil {
		log.Error(err, "Failed to read web identity token file")
		return nil, errors.Wrap(err, "failed to read web identity token file")
	}

	log.V(1).Info("Successfully read token from file")
	return &credentials.WebIdentityToken{
		Token: string(data),
	}, nil
}
