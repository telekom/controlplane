// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package metrics_test

import (
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	client "github.com/telekom/controlplane/common-server/pkg/client/metrics"
)

var _ client.HttpRequestDoer = &mockClient{}

type mockClient struct {
	do func(req *http.Request) (*http.Response, error)
}

func (m *mockClient) Do(req *http.Request) (*http.Response, error) {
	if m.do == nil {
		return &http.Response{
			StatusCode: http.StatusOK,
		}, nil
	}
	return m.do(req)
}

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Suite")
}
