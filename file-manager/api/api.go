// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/api/accesstoken"
	"github.com/telekom/controlplane/file-manager/api/gen"
	"github.com/telekom/controlplane/secret-manager/api/util"
	"net/http"
	"os"
	"strings"
)

const (
	localhost = "http://localhost:9090/api"
	inCluster = "https://secret-manager.secret-manager-system.svc.cluster.local/api"

	// empty string as ca path will let the function skip searching and configuring the certificates, which we dont want to use here
	disableCA = ""
)

var (
	ErrNotFound = errors.New("resource not found")
)

type DownloadApi interface {
}

type UploadApi interface {
}

type FileManager interface {
	UploadApi
	DownloadApi
}

var _ FileManager = (*FileManager)(nil)

type fileManagerAPI struct {
	client gen.ClientWithResponsesInterface
}

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

// todo - shared functionality - move to common ?
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

func New(opts ...Option) FileManager {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	if !strings.HasPrefix(options.URL, "https://") {
		fmt.Println("⚠️\tWarning: Using HTTP instead of HTTPS. This is not secure.")
	}
	skipTlsVerify := os.Getenv("SKIP_TLS_VERIFY") == "true" || options.SkipTLSVerify
	httpClient, err := gen.NewClientWithResponses(options.URL, gen.WithHTTPClient(util.NewHttpClientOrDie(skipTlsVerify, disableCA)), gen.WithRequestEditorFn(options.accessTokenReqEditor))
	if err != nil {
		panic(fmt.Sprintf("Failed to create client: %v", err))
	}
	return &fileManagerAPI{
		client: httpClient,
	}
}
