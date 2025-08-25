// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"os"

	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
)

type CredentialProvider string

const (
	CredentialProviderEnvMinio       CredentialProvider = "env-minio"
	CredentialProviderSTSWebIdentity CredentialProvider = "sts-web-identity"
	CredentialProviderStatic         CredentialProvider = "static"
)

type Properties interface {
	GetDefault(key string, defaultValue string) string
	Get(key string) string
}

type propertiesMap map[string]string

func (p propertiesMap) GetDefault(key string, defaultValue string) string {
	value, exists := p[key]
	if !exists {
		return defaultValue
	}
	return value
}

func (p propertiesMap) Get(key string) string {
	return p[key]
}

type CredentialOptions struct {
	properties Properties
}

func WithProperties(props Properties) CredentialOption {
	return func(o *CredentialOptions) {
		o.properties = props
	}
}

func WithProperty(key, value string) CredentialOption {
	return func(o *CredentialOptions) {
		if o.properties == nil {
			o.properties = propertiesMap{}
		}
		o.properties.(propertiesMap)[key] = value
	}
}

type CredentialOption func(*CredentialOptions)

func AutoDiscoverProvider(properties Properties) CredentialProvider {
	if properties.GetDefault("roleArn", os.Getenv("AWS_ROLE_ARN")) != "" {
		return CredentialProviderSTSWebIdentity
	}

	if properties.Get("accessKey") != "" && properties.Get("secretKey") != "" {
		return CredentialProviderStatic
	}
	return CredentialProviderEnvMinio
}

func NewCredentials(provider CredentialProvider, opts ...CredentialOption) (*credentials.Credentials, error) {
	options := &CredentialOptions{}
	for _, o := range opts {
		o(options)
	}

	switch provider {
	case CredentialProviderSTSWebIdentity:
		stsEndpoint := options.properties.GetDefault("stsEndpoint", "https://sts.amazonaws.com")
		roleArn := options.properties.GetDefault("roleArn", os.Getenv("AWS_ROLE_ARN"))
		tokenPath := options.properties.GetDefault("tokenPath", BucketsBackendTokenPath)
		staticToken := options.properties.GetDefault("token", os.Getenv("AWS_WEB_IDENTITY_TOKEN"))

		var token accesstoken.AccessToken
		if staticToken != "" {
			token = accesstoken.NewStaticAccessToken(staticToken)
		} else {
			token = accesstoken.NewAccessToken(tokenPath)
		}

		getWebIDTokenExpiry := func() (*credentials.WebIdentityToken, error) {
			tokenStr, err := token.Read()
			if err != nil {
				return nil, errors.Wrap(err, "failed to read access token from file")
			}
			return &credentials.WebIdentityToken{
				Token: tokenStr,
			}, nil
		}
		stsOpts := func(si *credentials.STSWebIdentity) {
			si.RoleARN = roleArn
		}

		creds, err := credentials.NewSTSWebIdentity(stsEndpoint, getWebIDTokenExpiry, stsOpts)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create STS web identity credentials")
		}
		return creds, nil

	case CredentialProviderEnvMinio:
		return credentials.New(&credentials.EnvMinio{}), nil

	case CredentialProviderStatic:
		accessKey := options.properties.Get("accessKey")
		secretKey := options.properties.Get("secretKey")
		if accessKey == "" || secretKey == "" {
			return nil, errors.New("static credentials require both accessKey and secretKey properties")
		}
		return credentials.NewStaticV4(accessKey, secretKey, ""), nil

	default:
		return nil, errors.Errorf("unknown credential provider type: %s", provider)
	}
}
