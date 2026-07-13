// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/organization-server/internal/client"
	"github.com/telekom/controlplane/organization-server/internal/handler"
	mw "github.com/telekom/controlplane/organization-server/internal/middleware"

	. "github.com/onsi/gomega"
)

// mockGraphQLServer returns an httptest.Server that responds to GraphQL requests
// with canned responses based on the operation name.
func mockGraphQLServer(responses map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			OperationName string `json:"operationName"`
		}
		_ = json.Unmarshal(body, &req)

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

	cpapiClient := client.NewCPAPIClient(graphqlURL, nil, "")
	roverClient := client.NewRoverClient(roverURL, "tardis")
	h := handler.New(cpapiClient, roverClient, logr.Discard())

	// In tests: mock JWT (no trusted issuers) + identity extraction + permissive team auth
	teamAuth := mw.TeamAuthorization(logr.Discard())
	api := app.Group("/organization/v1",
		mw.JWTValidation(logr.Discard(), nil),
		mw.IdentityExtraction(logr.Discard(), "test"),
		mw.Obfuscate(),
	)
	h.RegisterRoutes(api, teamAuth)

	return app
}

// makeToken creates a mock JWT (unsigned) with the given claims for testing.
// Uses the clientId format expected by IdentityExtraction: "group--team--user"
func makeToken(group, team string, scopes []string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	claims := map[string]any{
		"clientId": group + "--" + team + "--test-user",
		"scope":    strings.Join(scopes, " "),
		"exp":      time.Now().Add(time.Hour).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	return header + "." + payload + "."
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
