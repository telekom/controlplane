// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"os"

	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
)

const WebIdentityTokenEnvVar = "MC_WEB_IDENTITY_TOKEN"

// createTokenProvider creates a token provider function for a given token
func (c *BucketConfig) createTokenProvider(token string) func() (*credentials.WebIdentityToken, error) {
	return func() (*credentials.WebIdentityToken, error) {
		return &credentials.WebIdentityToken{
			Token: token,
		}, nil
	}
}

// createSTSCredentials creates STS web identity credentials using a token provider
func (c *BucketConfig) createSTSCredentials(tokenProvider func() (*credentials.WebIdentityToken, error)) (*credentials.Credentials, error) {
	c.Logger.V(1).Info("Creating STS web identity credentials")
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
		return nil, errors.Wrap(err, "failed to create STS web identity credentials")
	}
	return creds, nil
}

// getCredentials returns appropriate credentials based on token availability
func (c *BucketConfig) getCredentials(tokenAvailable bool, token string) (*credentials.Credentials, error) {
	if tokenAvailable {
		tokenProvider := c.createTokenProvider(token)
		return c.createSTSCredentials(tokenProvider)
	} else {
		// Create anonymous credentials as fallback
		c.Logger.V(0).Info("Using anonymous credentials - limited functionality may be available")
		return credentials.NewStatic("", "", "", credentials.SignatureAnonymous), nil
	}
}

// RefreshCredentialsOrDiscard checks if the token has changed and updates credentials if needed.
// If the token is unchanged or unavailable, does nothing.
func (c *BucketConfig) RefreshCredentialsOrDiscard() error {
	token, available := c.getTokenFromSources()
	if !available {
		c.Logger.V(1).Info("No token available, skipping credential refresh")
		return nil
	}
	if c.currentToken == token {
		c.Logger.V(1).Info("Token unchanged, skipping credentials update")
		return nil
	}
	c.Logger.V(1).Info("Token changed, updating credentials")
	c.currentToken = token
	creds, err := c.getCredentials(true, token)
	if err != nil {
		return err
	}
	c.currentCreds = creds
	if c.Client != nil {
		client, err := c.createMinioClient(creds)
		if err != nil {
			return errors.Wrap(err, "failed to create new client with updated credentials")
		}
		c.Client = client
		c.Logger.V(1).Info("Updated client with new credentials")
	}
	return nil
}

// getTokenFromSources tries to get a token from various sources (environment, file)
// Returns the token, a boolean indicating whether it's available, and any error
func (c *BucketConfig) getTokenFromSources() (string, bool) {
	log := c.Logger
	var token string
	var available bool

	// Try to get token from environment first
	if os.Getenv(WebIdentityTokenEnvVar) != "" {
		log.V(1).Info("Getting token from environment")
		webToken, err := c.getWebIDTokenFromEnv()
		if err == nil {
			token = webToken.Token
			available = true
			return token, available
		} else {
			log.V(1).Info("Failed to get token from environment", "error", err.Error())
		}
	}

	// If no token from environment, try from file
	log.V(1).Info("Trying to get token from file", "path", c.TokenPath)
	webToken, err := c.getWebIDTokenFromFile()
	if err == nil {
		token = webToken.Token
		available = true
		return token, available
	} else {
		log.V(1).Info("Failed to get token from file", "error", err.Error())
	}

	// No token found
	log.V(0).Info("No identity token found in environment or file")
	return "", false
}

// getWebIDTokenFromEnv retrieves the web identity token from an environment variable
func (c *BucketConfig) getWebIDTokenFromEnv() (*credentials.WebIdentityToken, error) {
	// Use the configured logger
	log := c.Logger

	log.V(1).Info("Retrieving web identity token from environment variable")
	data := os.Getenv(WebIdentityTokenEnvVar)
	if data == "" {
		log.Error(nil, WebIdentityTokenEnvVar+" environment variable not set")
		return nil, errors.New(WebIdentityTokenEnvVar + " environment variable not set")
	}

	log.V(1).Info("Successfully retrieved token from environment variable")
	return &credentials.WebIdentityToken{
		Token: data,
	}, nil
}

// getWebIDTokenFromFile retrieves the web identity token from a file
func (c *BucketConfig) getWebIDTokenFromFile() (*credentials.WebIdentityToken, error) {
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
