// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Khan/genqlient/graphql"

	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

func TestCPAPIClientForwardsBusinessContext(t *testing.T) {
	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"__typename":"Query"}}`))
	}))
	defer server.Close()

	ctx := security.ToContext(context.Background(), &security.BusinessContext{
		Environment: "dev",
		Group:       "eni",
		Team:        "hyperion",
	})

	client := NewCPAPIClient(server.URL, accesstoken.NewStaticAccessToken("token"), "")
	if err := client.MakeRequest(ctx, &graphql.Request{Query: "query Test { __typename }"}, &graphql.Response{}); err != nil {
		t.Fatalf("request failed: %v", err)
	}

	for name, want := range map[string]string{
		"X-Environment":           "dev",
		"X-Forwarded-Environment": "dev",
		"X-Forwarded-Group":       "eni",
		"X-Forwarded-Team":        "hyperion",
	} {
		if got := headers.Get(name); got != want {
			t.Errorf("%s: want %q, got %q", name, want, got)
		}
	}
}
