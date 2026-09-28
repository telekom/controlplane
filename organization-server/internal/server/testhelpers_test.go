// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"

	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	securitymock "github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
	"github.com/telekom/controlplane/organization-server/internal/client"
	"github.com/telekom/controlplane/organization-server/internal/controller"
	"github.com/telekom/controlplane/organization-server/internal/server"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// gqlRequest is a GraphQL request that mockGraphQLServer received.
type gqlRequest struct {
	OperationName string         `json:"operationName"`
	Variables     map[string]any `json:"variables"`
}

// gqlVariables returns the variables of the last received request for the operation.
// It fails the test if the operation was not received.
func gqlVariables(requests []gqlRequest, operationName string) map[string]any {
	for i := len(requests) - 1; i >= 0; i-- {
		if requests[i].OperationName == operationName {
			return requests[i].Variables
		}
	}
	Fail("GraphQL operation not received: "+operationName, 1)
	return nil
}

// mockGraphQLServer returns an httptest.Server that responds to GraphQL requests
// with canned responses based on the operation name.
// If requests is given, the server records every received request in it.
func mockGraphQLServer(responses map[string]any, requests ...*[]gqlRequest) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req gqlRequest
		_ = json.Unmarshal(body, &req)
		if len(requests) > 0 {
			*requests[0] = append(*requests[0], req)
		}

		resp, ok := responses[req.OperationName]
		if !ok {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"unknown operation: ` + req.OperationName + `"}]}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		data, _ := json.Marshal(map[string]any{"data": resp})
		_, _ = w.Write(data)
	}))
}

// mockRoverServer returns an httptest.Server that responds to rover REST calls.
func mockRoverServer(responses map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for pathPrefix, body := range responses {
			if strings.HasPrefix(r.URL.Path, pathPrefix) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

// newTestApp creates a Fiber app wired to the given mock servers.
func newTestApp(graphqlURL, roverURL string) *fiber.App {
	app := fiber.New(fiber.Config{
		JSONEncoder: sonic.Marshal,
		JSONDecoder: sonic.Unmarshal,
	})

	token := accesstoken.NewStaticAccessToken("test-token")
	cpapiClient := client.NewCPAPIClient(graphqlURL, token, "")
	roverClient := client.NewRoverClient(roverURL, token, "")
	ctrl := controller.New(cpapiClient, roverClient)
	srv := server.New(ctrl, logr.Discard())

	api := app.Group("/organization/v1")
	family := cserver.JWTFamily(security.SecurityOpts{
		Mode: security.ModeMock,
		Log:  logr.Discard(),
		BusinessContextOpts: []security.Option[*security.BusinessContextOpts]{
			security.WithScopePrefix("tardis:"),
		},
		CheckAccessOpts: []security.Option[*security.CheckAccessOpts]{
			security.WithPathParamKey("hub", "team"),
			security.WithTemplates(server.SecurityTemplates),
		},
	})
	srv.RegisterRoutes(api, family(api))

	return app
}

// makeToken creates a mock JWT with common-server's standard claims.
func makeToken(group, team string, scopes []string) string {
	return securitymock.NewMockAccessToken("test", group, team, scopes)
}

// makeTokenWithClaims creates a mock JWT with arbitrary claims.
func makeTokenWithClaims(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	return header + "." + payload + "."
}

// executeRequest sends an HTTP request through the test Fiber app.
func executeRequest(app *fiber.App, req *http.Request, token string) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return app.Test(req, -1)
}

// expectJSON asserts 200 OK with JSON body and returns the parsed body.
func expectJSON(resp *http.Response, err error) map[string]any {
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, resp.StatusCode).To(Equal(http.StatusOK))
	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	ExpectWithOffset(1, json.Unmarshal(body, &result)).To(Succeed())
	return result
}

// expectJSONArray asserts 200 OK with JSON array body.
func expectJSONArray(resp *http.Response, err error) []any {
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, resp.StatusCode).To(Equal(http.StatusOK))
	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	ExpectWithOffset(1, json.Unmarshal(body, &result)).To(Succeed())
	items, ok := result["items"].([]any)
	if !ok {
		return nil
	}
	return items
}

// expectStatus asserts a specific HTTP status code.
func expectStatus(resp *http.Response, err error, code int) {
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, resp.StatusCode).To(Equal(code))
}
