// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gofiber/fiber/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hub Handlers", func() {
	var (
		app        *fiber.App
		adminToken string
		obfToken   string
	)

	BeforeEach(func() {
		gqlServer := mockGraphQLServer(map[string]any{
			"ListGroups": map[string]any{
				"groups": []map[string]any{
					{
						"id":          "1",
						"name":        "eni",
						"displayName": "Eni Group",
						"description": "The Eni group",
						"teams":       []any{},
					},
					{
						"id":          "2",
						"name":        "cit",
						"displayName": "CIT Group",
						"description": "The CIT group",
						"teams":       []any{},
					},
				},
			},
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
			"CreateGroup": map[string]any{
				"createGroup": map[string]any{
					"group": map[string]any{
						"id":          "3",
						"name":        "new-hub",
						"displayName": "New Hub",
						"description": "A new hub",
						"teams":       []any{},
					},
					"errors": []any{},
				},
			},
			"UpdateGroup": map[string]any{
				"updateGroup": map[string]any{
					"group": map[string]any{
						"id":          "1",
						"name":        "eni",
						"displayName": "Updated Eni",
						"description": "Updated description",
						"teams":       []any{},
					},
					"errors": []any{},
				},
			},
			"DeleteGroup": map[string]any{
				"deleteGroup": map[string]any{
					"errors": []any{},
				},
			},
		})
		DeferCleanup(gqlServer.Close)

		roverServer := mockRoverServer(nil)
		DeferCleanup(roverServer.Close)

		app = newTestApp(gqlServer.URL, roverServer.URL)
		adminToken = makeToken("eni", "myteam", []string{"tardis:admin:all"})
		obfToken = makeToken("eni", "myteam", []string{"tardis:admin:obfuscated"})
	})

	Describe("GET /organization/v1/hubs", func() {
		It("should list all hubs", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			items := expectJSONArray(resp, err)
			Expect(items).To(HaveLen(2))
		})
	})

	Describe("GET /organization/v1/hubs/:hub", func() {
		It("should get a single hub", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			result := expectJSON(resp, err)
			Expect(result["name"]).To(Equal("eni"))
			Expect(result["displayName"]).To(Equal("Eni Group"))
		})
	})

	Describe("POST /organization/v1/hubs", func() {
		It("should create a hub", func() {
			body := `{"name":"new-hub","displayName":"New Hub","description":"A new hub"}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))
		})
	})

	Describe("PUT /organization/v1/hubs/:hub", func() {
		It("should update a hub", func() {
			body := `{"displayName":"Updated Eni","description":"Updated description"}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))
		})
	})

	Describe("DELETE /organization/v1/hubs/:hub", func() {
		It("should delete a hub", func() {
			req := httptest.NewRequest(http.MethodDelete, "/organization/v1/hubs/eni", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNoContent)
		})
	})

	Describe("Authentication", func() {
		It("should reject requests without a token", func() {
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs", http.NoBody)
			resp, err := app.Test(req, -1)
			expectStatus(resp, err, http.StatusUnauthorized)
		})
	})

	Describe("Obfuscation", func() {
		It("should not expose sensitive fields with obfuscated scope", func() {
			// Hub responses don't have sensitive fields, but verifies middleware doesn't break
			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni", http.NoBody)
			resp, err := executeRequest(app, req, obfToken)
			result := expectJSON(resp, err)
			Expect(result["name"]).To(Equal("eni"))
		})
	})
})
