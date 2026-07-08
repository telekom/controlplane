// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

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
		app        *fiber.App
		adminToken string
		obfToken   string
	)

	readyPhase := "READY"
	teamToken := "secret-token-value"
	now := time.Now().UTC().Truncate(time.Second)

	BeforeEach(func() {
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
		})
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
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams", nil)
			resp, err := executeRequest(app, req, adminToken)
			items := expectJSONArray(resp, err)
			Expect(items).To(HaveLen(1))
			team := items[0].(map[string]any)
			Expect(team["name"]).To(Equal("hyperion"))
		})
	})

	Describe("GET /organization/v1/hubs/:hub/teams/:team", func() {
		It("should get a team with full details for admin", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion", nil)
			resp, err := executeRequest(app, req, adminToken)
			result := expectJSON(resp, err)
			Expect(result["name"]).To(Equal("hyperion"))
			Expect(result["email"]).To(Equal("hyperion@telekom.de"))
			Expect(result["teamToken"]).To(Equal("secret-token-value"))
		})

		It("should strip teamToken for obfuscated scope", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion", nil)
			resp, err := executeRequest(app, req, obfToken)
			result := expectJSON(resp, err)
			Expect(result["name"]).To(Equal("hyperion"))
			Expect(result).NotTo(HaveKey("teamToken"))
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
			req := httptest.NewRequest(http.MethodDelete, "/organization/v1/hubs/eni/teams/hyperion", nil)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNoContent)
		})
	})

	Describe("PATCH /organization/v1/hubs/:hub/teams/:team/teamToken", func() {
		It("should rotate the team token", func() {
			req := httptest.NewRequest(http.MethodPatch, "/organization/v1/hubs/eni/teams/hyperion/teamToken", nil)
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, _ := io.ReadAll(resp.Body)
			var result map[string]any
			Expect(json.Unmarshal(body, &result)).To(Succeed())
			Expect(result["teamToken"]).To(Equal("new-rotated-token"))
		})

		It("should strip rotated token for obfuscated scope", func() {
			req := httptest.NewRequest(http.MethodPatch, "/organization/v1/hubs/eni/teams/hyperion/teamToken", nil)
			resp, err := executeRequest(app, req, obfToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, _ := io.ReadAll(resp.Body)
			var result map[string]any
			Expect(json.Unmarshal(body, &result)).To(Succeed())
			Expect(result).NotTo(HaveKey("teamToken"))
		})
	})

	Describe("GET /organization/v1/hubs/:hub/teams/:team/resources", func() {
		It("should proxy resources from rover-server", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion/resources", nil)
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})
})

func stringPtr(s string) *string {
	return &s
}
