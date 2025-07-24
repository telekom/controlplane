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

// UpdateBearerToken updates the current token and recreates the client credentials if the token has changed
// This should be called before each request to ensure the client has the latest token
func (c *S3Config) UpdateBearerToken(token string) error {
	// If token is unchanged, no need to update
	if c.currentToken == token {
		c.Logger.V(1).Info("Token unchanged, skipping credentials update")
		return nil
	}

	// Update the current token
	c.Logger.V(1).Info("Token changed, updating credentials")
	c.currentToken = token

	// Create a token provider function that returns the provided bearer token
	tokenProvider := func() (*credentials.WebIdentityToken, error) {
		return &credentials.WebIdentityToken{
			Token: token,
		}, nil
	}

	// Create credentials with the new token
	creds, err := credentials.NewSTSWebIdentity(
		c.STSEndpoint,
		tokenProvider,
		func(i *credentials.STSWebIdentity) {
			i.RoleARN = c.RoleSessionArn
			c.Logger.V(1).Info("Setting role ARN for STS", "roleARN", i.RoleARN)
		},
	)
	if err != nil {
		c.Logger.Error(err, "Failed to create STS web identity credentials")
		return errors.Wrap(err, "failed to create STS web identity credentials")
	}

	// Store the new credentials
	c.currentCreds = creds

	// For the minio S3 client, we need to recreate it with the new credentials
	if c.Client != nil {
		client, err := minio.New(c.Endpoint, &minio.Options{
			Creds:  creds,
			Secure: true,
			// TODO: CHECKSUM-SHA-256: Enable SHA-256 checksum calculation and verification
			//TrailingHeaders: true, // Enable trailing headers for checksum support
		})
		if err != nil {
			c.Logger.Error(err, "Failed to create new S3 client with updated credentials")
			return errors.Wrap(err, "failed to create new S3 client with updated credentials")
		}

		// Replace the client
		c.Client = client
		c.Logger.V(1).Info("Updated client with new credentials")
	}

	return nil
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

	// Get initial token for client creation if possible
	// This is just for initial setup; we'll update with real bearer tokens later
	var initialToken string
	var tokenAvailable bool
	var err error

	// Try to get token from environment first
	if os.Getenv("MC_WEB_IDENTITY_TOKEN") != "" {
		log.V(1).Info("Getting initial token from environment for client setup")
		token, err := c.getWebIDTokenFromEnv()
		if err == nil {
			initialToken = token.Token
			tokenAvailable = true
		} else {
			log.V(1).Info("Failed to get token from environment, will try file next", "error", err.Error())
		}
	}

	// If no token from environment, try from file
	if !tokenAvailable {
		log.V(1).Info("Trying to get initial token from file", "path", c.TokenPath)
		token, err := c.getWebIDTokenFromFile()
		if err == nil {
			initialToken = token.Token
			tokenAvailable = true
		} else {
			log.V(1).Info("Failed to get token from file", "error", err.Error())
		}
	}

	// Store the initial token if available
	if tokenAvailable {
		c.currentToken = initialToken
	} else {
		log.V(0).Info("No identity token found in environment or file - will use anonymous credentials")
	}

	// Create credentials based on whether a token is available
	var creds *credentials.Credentials

	if tokenAvailable {
		// Define the token provider function that returns our current token
		tokenProvider := func() (*credentials.WebIdentityToken, error) {
			return &credentials.WebIdentityToken{
				Token: c.currentToken,
			}, nil
		}

		// Create STS web identity credentials
		log.V(1).Info("Creating STS web identity credentials")
		creds, err = credentials.NewSTSWebIdentity(
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
	} else {
		// Create anonymous credentials as fallback
		log.V(0).Info("Using anonymous credentials - limited functionality may be available")
		creds = credentials.NewStatic("", "", "", credentials.SignatureAnonymous)
	}

	// Store the credentials
	c.currentCreds = creds

	// Create S3 client
	log.V(1).Info("Creating Minio S3 client", "endpoint", c.Endpoint)
	client, err := minio.New(c.Endpoint, &minio.Options{
		Creds:  creds,
		Secure: true,
		// TODO: CHECKSUM-SHA-256: Enable SHA-256 checksum calculation and verification
		//TrailingHeaders: true, // Enable trailing headers for checksum support
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

	// If token path is empty, return an error
	if c.TokenPath == "" {
		log.V(1).Info("No token file path specified")
		return nil, errors.New("no token file path specified")
	}

	log.V(1).Info("Reading web identity token from file", "path", c.TokenPath)

	// Check if file exists before trying to read it
	if _, err := os.Stat(c.TokenPath); os.IsNotExist(err) {
		log.V(1).Info("Token file does not exist", "path", c.TokenPath)
		return nil, errors.New("token file does not exist")
	}

	data, err := os.ReadFile(c.TokenPath)
	if err != nil {
		log.V(1).Info("Failed to read web identity token file", "error", err.Error())
		return nil, errors.Wrap(err, "failed to read web identity token file")
	}

	// Check if file is empty
	if len(data) == 0 {
		log.V(1).Info("Token file is empty", "path", c.TokenPath)
		return nil, errors.New("token file is empty")
	}

	log.V(1).Info("Successfully read token from file")
	return &credentials.WebIdentityToken{
		Token: string(data),
	}, nil
}
