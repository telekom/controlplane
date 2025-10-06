// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/client"
	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
	"github.com/telekom/controlplane/common-server/pkg/util"
	"github.com/telekom/controlplane/secret-manager/api/gen"
)

type Options struct {
	URL           string
	Token         accesstoken.AccessToken
	SkipTLSVerify bool
}

func (o *Options) accessTokenReqEditor(ctx context.Context, req *http.Request) error {
	if o.Token == nil {
		return errors.New("access token is not set")
	}
	token, err := o.Token.Read()
	if err != nil {
		return errors.Wrap(err, "failed to read access token")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return nil
}

func defaultOptions() *Options {
	if util.IsRunningInCluster() {
		return &Options{
			URL:           inCluster,
			Token:         accesstoken.NewAccessToken(TokenFilePath),
			SkipTLSVerify: false,
		}
	} else {
		tokenEnv := os.Getenv("SECRET_MANAGER_TOKEN")
		if tokenEnv == "" {
			log.Fatal("SECRET_MANAGER_TOKEN environment variable is not set")
		}
		return &Options{
			URL:           localhost,
			Token:         accesstoken.NewStaticAccessToken(tokenEnv),
			SkipTLSVerify: true,
		}
	}
}

type Option func(*Options)

func WithURL(url string) Option {
	return func(o *Options) {
		o.URL = url
	}
}

func WithAccessToken(token accesstoken.AccessToken) Option {
	return func(o *Options) {
		o.Token = token
	}
}

func WithSkipTLSVerify() Option {
	return func(o *Options) {
		o.SkipTLSVerify = true
	}
}

func NewOnboarding(opts ...Option) OnboardingApi {
	return New(opts...)
}

func NewSecrets(opts ...Option) SecretsApi {
	return New(opts...)
}

func New(opts ...Option) SecretManager {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	if !strings.HasPrefix(options.URL, "https://") {
		fmt.Println("⚠️\tWarning: Using HTTP instead of HTTPS. This is not secure.")
	}
	skipTlsVerify := os.Getenv("SKIP_TLS_VERIFY") == "true" || options.SkipTLSVerify
	httpClient, err := gen.NewClientWithResponses(options.URL, gen.WithHTTPClient(
		client.NewHttpClientOrDie(
			client.WithSkipTlsVerify(skipTlsVerify),
			client.WithCaFilepath(CaFilePath),
			client.WithClientName("secret-manager"),
			client.WithReplacePattern(`^\/api\/v1\/(secrets|onboarding)\/(?P<redacted>.*)$`),
		),
	),
		gen.WithRequestEditorFn(options.accessTokenReqEditor),
	)
	if err != nil {
		log.Fatalf("Failed to create HTTP client: %v", err)
	}
	return &secretManagerAPI{
		client: httpClient,
	}
}
