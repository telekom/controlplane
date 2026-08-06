// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Team Handlers", func() {
	var (
		app           *fiber.App
		adminToken    string
		obfToken      string
		gqlOperations []string
	)

	readyPhase := "READY"
	teamToken := "secret-token-value"
	now := time.Now().UTC().Truncate(time.Second)

	BeforeEach(func() {
		gqlOperations = nil
		gqlServer := mockGraphQLServer(map[string]any{
			"GetGroup": map[string]any{
				"groups": []map[string]any{
					{
						"id":          "1",
						"name":        "eni",
						"displayName": "Eni Group",
						"description": "The Eni group",
						"teams":       []any{},
					},
				},
			},
			"ListTeams": map[string]any{
				"teams": map[string]any{
					"edges": []map[string]any{
						{
							"node": map[string]any{
								"id":             "10",
								"name":           "eni--hyperion",
								"email":          "hyperion@telekom.de",
								"createdAt":      now.Format(time.RFC3339),
								"lastModifiedAt": now.Format(time.RFC3339),
								"statusPhase":    &readyPhase,
								"teamToken":      &teamToken,
								"group":          map[string]any{"name": "eni"},
								"members": []map[string]any{
									{"name": "Alice", "email": "alice@telekom.de"},
								},
							},
						},
						{
							"node": map[string]any{
								"id":             "11",
								"name":           "eni--other-team",
								"email":          "other@telekom.de",
								"createdAt":      now.Format(time.RFC3339),
								"lastModifiedAt": now.Format(time.RFC3339),
								"statusPhase":    &readyPhase,
								"teamToken":      &teamToken,
								"group":          map[string]any{"name": "eni"},
								"members":        []any{},
							},
						},
					},
				},
			},
			"GetTeam": map[string]any{
				"teams": map[string]any{
					"edges": []map[string]any{
						{
							"node": map[string]any{
								"id":             "10",
								"name":           "hyperion",
								"email":          "hyperion@telekom.de",
								"createdAt":      now.Format(time.RFC3339),
								"lastModifiedAt": now.Format(time.RFC3339),
								"statusPhase":    &readyPhase,
								"teamToken":      &teamToken,
								"group":          map[string]any{"name": "eni"},
								"members": []map[string]any{
									{"name": "Alice", "email": "alice@telekom.de"},
								},
							},
						},
					},
				},
			},
			"CreateTeam": map[string]any{
				"createTeam": map[string]any{
					"team": map[string]any{
						"id":             "11",
						"name":           "newteam",
						"email":          "new@telekom.de",
						"createdAt":      now.Format(time.RFC3339),
						"lastModifiedAt": now.Format(time.RFC3339),
						"statusPhase":    nil,
						"teamToken":      &teamToken,
						"group":          map[string]any{"name": "eni"},
						"members":        []any{},
					},
					"errors": []any{},
				},
			},
			"UpdateTeam": map[string]any{
				"updateTeam": map[string]any{
					"team": map[string]any{
						"id":             "10",
						"name":           "hyperion",
						"email":          "updated@telekom.de",
						"createdAt":      now.Format(time.RFC3339),
						"lastModifiedAt": now.Format(time.RFC3339),
						"statusPhase":    &readyPhase,
						"teamToken":      &teamToken,
						"group":          map[string]any{"name": "eni"},
						"members":        []any{},
					},
					"errors": []any{},
				},
			},
			"DeleteTeam": map[string]any{
				"deleteTeam": map[string]any{
					"errors": []any{},
				},
			},
			"RotateTeamToken": map[string]any{
				"rotateTeamToken": map[string]any{
					"team": map[string]any{
						"id":             "10",
						"name":           "hyperion",
						"email":          "hyperion@telekom.de",
						"createdAt":      now.Format(time.RFC3339),
						"lastModifiedAt": now.Format(time.RFC3339),
						"statusPhase":    &readyPhase,
						"teamToken":      stringPtr("new-rotated-token"),
						"group":          map[string]any{"name": "eni"},
						"members":        []any{},
					},
					"errors": []any{},
				},
			},
		}, &gqlOperations)
		DeferCleanup(gqlServer.Close)

		roverServer := mockRoverServer(map[string]string{
			"/resources": `{"items":[{"name":"my-resource","kind":"ApiExposure","apiVersion":"v1","path":"/apis/my-resource"}]}`,
		})
		DeferCleanup(roverServer.Close)

		app = newTestApp(gqlServer.URL, roverServer.URL)
		adminToken = makeToken("eni", "hyperion", []string{"tardis:admin:all"})
		obfToken = makeToken("eni", "hyperion", []string{"tardis:admin:obfuscated"})
	})

	Describe("GET /organization/v1/hubs/:hub/teams", func() {
		It("should list teams", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			items := expectJSONArray(resp, err)
			Expect(items).To(HaveLen(2))
			team := items[0].(map[string]any)
			Expect(team["name"]).To(Equal("hyperion"))
		})

		It("filters team clients before pagination", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams?limit=1", http.NoBody)
			resp, err := executeRequest(app, req, makeToken("eni", "hyperion", []string{"tardis:team:read"}))
			items := expectJSONArray(resp, err)
			Expect(items).To(HaveLen(1))
			Expect(items[0].(map[string]any)["name"]).To(Equal("hyperion"))
			Expect(resp.Header.Get("X-Total-Count")).To(Equal("1"))
			Expect(resp.Header.Get("X-Result-Count")).To(Equal("1"))
		})

		It("allows legacy hub:read", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams", http.NoBody)
			resp, err := executeRequest(app, req, makeToken("eni", "hyperion", []string{"tardis:hub:read"}))
			items := expectJSONArray(resp, err)
			Expect(items).To(HaveLen(2))
		})
	})

	Describe("GET /organization/v1/hubs/:hub/teams/:team", func() {
		It("should get a team with full details for admin", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			result := expectJSON(resp, err)
			Expect(result["name"]).To(Equal("hyperion"))
			Expect(result["email"]).To(Equal("hyperion@telekom.de"))
			Expect(result["teamToken"]).To(Equal("secret-token-value"))
		})

		It("should strip teamToken for obfuscated scope", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion", http.NoBody)
			resp, err := executeRequest(app, req, obfToken)
			result := expectJSON(resp, err)
			Expect(result["name"]).To(Equal("hyperion"))
			Expect(result).NotTo(HaveKey("teamToken"))
		})

		It("should allow admin token from a different hub", func() {
			crossHubAdmin := makeToken("platform", "service", []string{"tardis:admin:all"})
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion", http.NoBody)
			resp, err := executeRequest(app, req, crossHubAdmin)
			result := expectJSON(resp, err)
			Expect(result["name"]).To(Equal("hyperion"))
		})
	})

	Describe("POST /organization/v1/hubs/:hub/teams", func() {
		It("should create a team", func() {
			body := `{"name":"newteam","email":"new@telekom.de","members":[]}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs/eni/teams", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))
		})
	})

	Describe("PUT /organization/v1/hubs/:hub/teams/:team", func() {
		It("should update a team", func() {
			body := `{"email":"updated@telekom.de","members":[]}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/hyperion", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))
		})
	})

	Describe("DELETE /organization/v1/hubs/:hub/teams/:team", func() {
		It("should delete a team", func() {
			req := httptest.NewRequest(http.MethodDelete, "/organization/v1/hubs/eni/teams/hyperion", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNoContent)
		})
	})

	Describe("PATCH /organization/v1/hubs/:hub/teams/:team/teamToken", func() {
		It("should rotate the team token", func() {
			req := httptest.NewRequest(http.MethodPatch, "/organization/v1/hubs/eni/teams/hyperion/teamToken", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, _ := io.ReadAll(resp.Body)
			var result map[string]any
			Expect(json.Unmarshal(body, &result)).To(Succeed())
			Expect(result["teamToken"]).To(Equal("new-rotated-token"))
		})

		It("should reject token rotation for obfuscated scope", func() {
			req := httptest.NewRequest(http.MethodPatch, "/organization/v1/hubs/eni/teams/hyperion/teamToken", http.NoBody)
			resp, err := executeRequest(app, req, obfToken)
			expectStatus(resp, err, http.StatusForbidden)
			Expect(gqlOperations).NotTo(ContainElement("RotateTeamToken"))
		})
	})

	Describe("GET /organization/v1/hubs/:hub/teams/:team/resources", func() {
		It("should proxy resources from rover-server", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion/resources", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("Authorization", func() {
		DescribeTable("enforces resource ownership boundaries",
			func(tokenGroup, tokenTeam, scope, path string, expectedStatus int) {
				req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
				resp, err := executeRequest(app, req, makeToken(tokenGroup, tokenTeam, []string{scope}))
				expectStatus(resp, err, expectedStatus)
			},
			Entry("allows exact team", "eni", "hyperion", "tardis:team:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusOK),
			Entry("rejects shorter token team", "eni", "hyper", "tardis:team:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusForbidden),
			Entry("rejects longer token team", "eni", "hyperionship", "tardis:team:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusForbidden),
			Entry("rejects shorter requested team", "eni", "hyperion", "tardis:team:read", "/organization/v1/hubs/eni/teams/hyper", http.StatusForbidden),
			Entry("rejects longer requested team", "eni", "hyperion", "tardis:team:read", "/organization/v1/hubs/eni/teams/hyperionship", http.StatusForbidden),
			Entry("rejects different team", "eni", "jupiter", "tardis:team:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusForbidden),
			Entry("rejects shorter token hub", "en", "hyperion", "tardis:team:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusForbidden),
			Entry("rejects longer token hub", "enigma", "hyperion", "tardis:team:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusForbidden),
			Entry("rejects longer requested hub", "eni", "hyperion", "tardis:team:read", "/organization/v1/hubs/enigma/teams/hyperion", http.StatusForbidden),
			Entry("allows case-only differences", "ENI", "HYPERION", "tardis:team:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusOK),
			Entry("uses token team when path omits team", "eni", "hyperion", "tardis:team:read", "/organization/v1/hubs/eni/teams", http.StatusOK),
			Entry("allows group within exact hub", "eni", "ignored", "tardis:group:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusOK),
			Entry("rejects shorter group hub", "en", "ignored", "tardis:group:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusForbidden),
			Entry("rejects longer group hub", "enigma", "ignored", "tardis:group:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusForbidden),
			Entry("allows admin across hubs", "platform", "service", "tardis:admin:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusOK),
			Entry("rejects unsupported scope", "eni", "hyperion", "tardis:unknown:read", "/organization/v1/hubs/eni/teams/hyperion", http.StatusForbidden),
		)

		It("should reject a missing token", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion", http.NoBody)
			resp, err := app.Test(req, -1)
			expectStatus(resp, err, http.StatusUnauthorized)
		})

		DescribeTable("enforces mutation access type",
			func(scope string, expectedStatus int) {
				body := `{"email":"updated@telekom.de","members":[]}`
				req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/hyperion", strings.NewReader(body))
				resp, err := executeRequest(app, req, makeToken("eni", "hyperion", []string{scope}))
				expectStatus(resp, err, expectedStatus)
			},
			Entry("rejects team read scope", "tardis:team:read", http.StatusForbidden),
			Entry("allows team all scope", "tardis:team:all", http.StatusAccepted),
		)

		DescribeTable("team creation requires admin:all",
			func(scope string) {
				body := `{"name":"newteam","email":"new@telekom.de","members":[]}`
				req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs/eni/teams", strings.NewReader(body))
				resp, err := executeRequest(app, req, makeToken("eni", "hyperion", []string{scope}))
				expectStatus(resp, err, http.StatusForbidden)
			},
			Entry("team client", "tardis:team:all"),
			Entry("group client", "tardis:group:all"),
			Entry("read-only admin client", "tardis:admin:read"),
		)

		It("should reject cross-team access for non-admin tokens", func() {
			teamToken := makeToken("eni", "other-team", []string{"tardis:team:all"})
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion", http.NoBody)
			resp, err := executeRequest(app, req, teamToken)
			expectStatus(resp, err, http.StatusForbidden)
		})

		It("should reject cross-hub access for non-admin tokens", func() {
			otherHubToken := makeToken("other-hub", "hyperion", []string{"tardis:team:all"})
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion", http.NoBody)
			resp, err := executeRequest(app, req, otherHubToken)
			expectStatus(resp, err, http.StatusForbidden)
		})
	})
})

func stringPtr(s string) *string {
	return &s
}
