// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

var (
	KubernetesWellKnownConfig = "https://kubernetes.default.svc/.well-known/openid-configuration"
)

type clusterInfo struct {
	Issuer  string `json:"issuer"`
	JwksUri string `json:"jwks_uri"`
}

func getClusterInfo() (c clusterInfo, err error) {
	client, err := GetKubernetesHttpClient()
	if err != nil {
		return c, errors.Wrap(err, "failed to get Kubernetes HTTP client")
	}
	res, err := client.Get(KubernetesWellKnownConfig)
	if err != nil {
		return c, errors.Wrap(err, "failed to perform HTTP request to Kubernetes API")
	}
	defer res.Body.Close() //nolint:errcheck

	if res.StatusCode != http.StatusOK {
		return c, fmt.Errorf("unexpected status code %d from Kubernetes API: %s", res.StatusCode, res.Status)
	}

	if err := json.NewDecoder(res.Body).Decode(&c); err != nil {
		return c, errors.Wrap(err, "failed to decode response body")
	}

	return c, nil
}
