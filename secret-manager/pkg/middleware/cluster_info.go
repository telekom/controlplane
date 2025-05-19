// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/secret-manager/api/util"
)

var (
	KubernetesWellKnownConfig   = "https://kubernetes.default.svc/.well-known/openid-configuration"
	ServiceAccountTokenFilepath = "/var/run/secrets/kubernetes.io/serviceaccount/token" //nolint:gosec
	ServiceAccountCAFilepath    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

type clusterInfo struct {
	Issuer  string `json:"issuer"`
	JwksUri string `json:"jwks_uri"`
}

func getClusterInfo() (c clusterInfo, err error) {
	token, err := os.ReadFile(ServiceAccountTokenFilepath)
	if err != nil {
		return c, errors.Wrap(err, "failed to read service account token")
	}
	req, _ := http.NewRequest(http.MethodGet, KubernetesWellKnownConfig, nil)
	req.Header.Set("Authorization", "Bearer "+string(token))
	req.Header.Set("Accept", "application/json")

	caPool, err := util.GetCert(ServiceAccountCAFilepath)
	if err != nil {
		return c, errors.Wrap(err, "failed to get CA pool")
	}
	client := http.Client{
		Timeout: 1 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS13,
				InsecureSkipVerify: false,
				RootCAs:            caPool,
			},
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return c, errors.Wrap(err, "failed to perform HTTP request to Kubernetes API")
	}
	defer res.Body.Close() //nolint:errcheck

	if res.StatusCode != http.StatusOK {
		return c, errors.New("failed to get cluster issuer")
	}

	if err := json.NewDecoder(res.Body).Decode(&c); err != nil {
		return c, errors.Wrap(err, "failed to decode response body")
	}

	return c, nil
}
