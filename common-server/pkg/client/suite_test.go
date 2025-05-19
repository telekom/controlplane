package client_test

import (
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/client"
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
