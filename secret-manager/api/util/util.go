// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	"github.com/telekom/controlplane/common-server/pkg/client"
)

var (
	ClientName     = "secret-manager"
	ReplacePattern = `^\/api\/v1\/(secrets|onboarding)\/(?P<redacted>.*)$`

	DefaultClientTimeout = 5 * time.Second
	ClientTimeout        = os.Getenv("CLIENT_TIMEOUT")
)

// IsRunningInCluster checks if the application is running in a Kubernetes cluster
func IsRunningInCluster() bool {
	_, ok := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	return ok
}

func NewHttpClientOrDie(skipTlsVerify bool, caFilepath string) client.HttpRequestDoer {
	var caPool *x509.CertPool

	if skipTlsVerify {
		fmt.Println("⚠️\tWarning: Using InsecureSkipVerify. This is not secure.")
	}

	if !skipTlsVerify && caFilepath != "" {
		certRefresher := NewCertRefresher(caFilepath)
		err := certRefresher.Start(context.Background())
		if err != nil {
			panic(err)
		}
		caPool = certRefresher.Pool
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTlsVerify,
			MinVersion:         tls.VersionTLS13,
			RootCAs:            caPool,
		},
	}
	timeout := DefaultClientTimeout
	if ClientTimeout != "" {
		var err error
		timeout, err = time.ParseDuration(ClientTimeout)
		if err != nil {
			panic(errors.Wrap(err, "failed to parse CLIENT_TIMEOUT"))
		}
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	return client.WithMetrics(httpClient, ClientName, ReplacePattern)
}

func GetCert(filepath string) (*x509.CertPool, error) {
	caCert, err := os.ReadFile(filepath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read CA certificate")
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to append CA certificate to pool")
	}
	return caCertPool, nil
}

type CertRefresher struct {
	Pool     *x509.CertPool
	filepath string
	lastCert []byte
	internal time.Duration
}

func NewCertRefresher(filepath string) *CertRefresher {
	return &CertRefresher{
		filepath: filepath,
		internal: 30 * time.Second,
	}
}

func (c *CertRefresher) Start(ctx context.Context) (err error) {
	c.Pool, err = GetCert(c.filepath)
	if err != nil {
		return errors.Wrap(err, "failed to start cert refresher")
	}
	go c.Watch(ctx)

	return nil
}

func (c *CertRefresher) Watch(ctx context.Context) {
	ticker := time.NewTicker(c.internal)
	defer ticker.Stop()

	log := logr.FromContextOrDiscard(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			caCert, err := os.ReadFile(c.filepath)
			if err != nil {
				log.Error(err, "failed to read cert file")
				continue
			}
			if bytes.Equal(caCert, c.lastCert) {
				log.V(1).Info("cert not changed")
				continue
			}

			if !c.Pool.AppendCertsFromPEM(caCert) {
				log.Info("failed to append certs from PEM")
				continue
			}
			c.lastCert = caCert
			log.V(1).Info("cert updated")
		}
	}
}
