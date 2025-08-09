// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"sync"
	"time"

	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
	"github.com/telekom/controlplane/common-server/pkg/util"

	"github.com/pkg/errors"
)

var (
	ServiceAccountTokenFilepath = "/var/run/secrets/kubernetes.io/serviceaccount/token" //nolint:gosec
	ServiceAccountCAFilepath    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

type AccessTokenTransport struct {
	Token     accesstoken.AccessToken
	Transport http.RoundTripper
}

func (ct *AccessTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if ct.Token != nil {
		token, err := ct.Token.Read()
		if err != nil {
			return nil, errors.Wrap(err, "failed to read access token")
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return ct.Transport.RoundTrip(req)
}

var initOnce sync.Once
var k8sHttpClient *http.Client

// GetKubernetesHttpClient initializes and returns a Kubernetes HTTP client
// using the service account token and CA certificate from the Kubernetes environment.
// It ensures that the client is only initialized once, even if called multiple times.
func GetKubernetesHttpClient() (*http.Client, error) {
	var err error
	initOnce.Do(func() {
		var caPool *x509.CertPool
		accessToken := accesstoken.NewAccessToken(ServiceAccountTokenFilepath)

		caPool, err = util.GetCert(ServiceAccountCAFilepath)
		if err != nil {
			err = errors.Wrap(err, "failed to read service account CA certificate")
			return
		}
		k8sHttpClient = &http.Client{
			Timeout: 5 * time.Second,
			Transport: &AccessTokenTransport{
				Token: accessToken,
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						MinVersion:         tls.VersionTLS13,
						InsecureSkipVerify: false,
						RootCAs:            caPool,
					},
				},
			},
		}
	})

	return k8sHttpClient, err
}
