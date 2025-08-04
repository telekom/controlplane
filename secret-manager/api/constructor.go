// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/secret-manager/api/accesstoken"
	"github.com/telekom/controlplane/secret-manager/api/gen"
	"github.com/telekom/controlplane/secret-manager/api/util"
)

type Options struct {
	URL           string
	Token         accesstoken.AccessToken
	SkipTLSVerify bool
}

func (o *Options) accessTokenReqEditor(ctx context.Context, req *http.Request) error {
	if o.Token == nil {
		return nil
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
			URL:   inCluster,
			Token: accesstoken.NewAccessToken(accesstoken.TokenFilePath),
		}
	} else {
		return &Options{
			URL:   localhost,
			Token: nil,
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
	httpClient, err := gen.NewClientWithResponses(options.URL, gen.WithHTTPClient(util.NewHttpClientOrDie(skipTlsVerify, CaFilePath)), gen.WithRequestEditorFn(options.accessTokenReqEditor))
	if err != nil {
		panic(fmt.Sprintf("Failed to create client: %v", err))
	}
	return &secretManagerAPI{
		client: httpClient,
	}
}
