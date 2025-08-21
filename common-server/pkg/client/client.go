// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	client "github.com/telekom/controlplane/common-server/pkg/client/metrics"
)

var (
	EnvClientTimeout = os.Getenv("CLIENT_TIMEOUT")
)

func NewHttpClientOrDie(opts ...Option) client.HttpRequestDoer {
	options := &Options{
		ClientName:    "http-client",
		ClientTimeout: 5 * time.Second,
	}
	for _, o := range opts {
		o(options)
	}

	var caPool *x509.CertPool

	if options.SkipTlsVerify {
		fmt.Println("⚠️\tWarning: Using InsecureSkipVerify. This is not secure.")
	}

	if !options.SkipTlsVerify && options.CaFilepath != "" {
		certRefresher := NewCertRefresher(options.CaFilepath)
		err := certRefresher.Start(context.Background())
		if err != nil {
			log.Fatalf("Failed to start cert refresher: %v", err)
		}
		caPool = certRefresher.Pool
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: options.SkipTlsVerify,
			MinVersion:         tls.VersionTLS13,
			RootCAs:            caPool,
		},
	}

	timeout := options.ClientTimeout
	if EnvClientTimeout != "" {
		var err error
		timeout, err = time.ParseDuration(EnvClientTimeout)
		if err != nil {
			log.Fatalf("Invalid CLIENT_TIMEOUT value: %v", err)
		}
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	return client.WithMetrics(httpClient,
		client.WithClientName(options.ClientName),
		client.WithReplaceFunc(client.NewReplacePath(regexp.MustCompile(options.ReplacePattern))),
	)
}
