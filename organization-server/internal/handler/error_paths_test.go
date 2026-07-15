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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hub Error Paths", func() {
	var adminToken string

	BeforeEach(func() {
		adminToken = makeToken("eni", "myteam", []string{"tardis:admin:all"})
	})

	Describe("CreateHub", func() {
		It("should return 400 for invalid JSON body", func() {
			gqlServer := mockGraphQLServer(nil)
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs", strings.NewReader("not-json"))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusBadRequest)
		})

		It("should return 502 when GQL server is down", func() {
			// Use a server that immediately closes
			gqlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			}))
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"name":"test","displayName":"Test","description":"desc"}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusInternalServerError)
		})

		It("should map mutation errors correctly", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"CreateGroup": map[string]any{
					"createGroup": map[string]any{
						"group": nil,
						"errors": []map[string]any{{
							"code":    "CONFLICT",
							"message": "Group already exists",
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"name":"existing","displayName":"Existing","description":"dup"}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusConflict)
		})
	})

	Describe("UpdateHub", func() {
		It("should return 400 for invalid JSON body", func() {
			gqlServer := mockGraphQLServer(nil)
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni", strings.NewReader("{bad"))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusBadRequest)
		})

		It("should return 404 when hub does not exist", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetGroup": map[string]any{
					"groups": []any{},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"displayName":"Updated","description":"updated"}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/nonexistent", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNotFound)
		})

		It("should map FORBIDDEN mutation errors", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetGroup": map[string]any{
					"groups": []map[string]any{{
						"id": "1", "name": "eni", "displayName": "Eni", "description": "",
					}},
				},
				"UpdateGroup": map[string]any{
					"updateGroup": map[string]any{
						"group": nil,
						"errors": []map[string]any{{
							"code":    "FORBIDDEN",
							"message": "Not allowed",
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"displayName":"Updated","description":"updated"}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusForbidden)
		})
	})

	Describe("DeleteHub", func() {
		It("should return 404 when hub not found", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetGroup": map[string]any{"groups": []any{}},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodDelete, "/organization/v1/hubs/nonexistent", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNotFound)
		})

		It("should map NOT_FOUND mutation errors", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetGroup": map[string]any{
					"groups": []map[string]any{{
						"id": "1", "name": "eni", "displayName": "Eni", "description": "",
					}},
				},
				"DeleteGroup": map[string]any{
					"deleteGroup": map[string]any{
						"errors": []map[string]any{{
							"code":    "NOT_FOUND",
							"message": "Already deleted",
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodDelete, "/organization/v1/hubs/eni", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNotFound)
		})
	})

	Describe("GetHub - not found", func() {
		It("should return 404 when hub not found", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetGroup": map[string]any{"groups": []any{}},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/nonexistent", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNotFound)
		})
	})

	Describe("GetHubStatus", func() {
		It("should return static done status", func() {
			gqlServer := mockGraphQLServer(nil)
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/status", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			result := expectJSON(resp, err)
			Expect(result["overallStatus"]).To(Equal("done"))
			Expect(result["processingState"]).To(Equal("done"))
			Expect(result["state"]).To(Equal("complete"))
		})
	})

	Describe("ListHubs with pagination", func() {
		It("should paginate results", func() {
			groups := make([]map[string]any, 5)
			for i := range groups {
				groups[i] = map[string]any{
					"id":          string(rune('1' + i)),
					"name":        "group-" + string(rune('a'+i)),
					"displayName": "Group " + string(rune('A'+i)),
					"description": "Desc",
					"teams":       []any{},
				}
			}

			gqlServer := mockGraphQLServer(map[string]any{
				"ListGroups": map[string]any{"groups": groups},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs?offset=1&limit=2", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, _ := io.ReadAll(resp.Body)
			var result map[string]any
			Expect(json.Unmarshal(body, &result)).To(Succeed())

			items := result["items"].([]any)
			Expect(items).To(HaveLen(2))

			paging := result["paging"].(map[string]any)
			Expect(paging["total"]).To(BeNumerically("==", 5))
		})
	})
})

var _ = Describe("Team Error Paths", func() {
	var adminToken string

	now := time.Now().UTC().Truncate(time.Second)

	BeforeEach(func() {
		adminToken = makeToken("eni", "hyperion", []string{"tardis:admin:all"})
	})

	Describe("CreateTeam", func() {
		It("should return 400 for invalid JSON body", func() {
			gqlServer := mockGraphQLServer(nil)
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs/eni/teams", strings.NewReader("bad"))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusBadRequest)
		})

		It("should return 502 when GQL server fails", func() {
			gqlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			}))
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"name":"newteam","email":"t@test.de","members":[]}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs/eni/teams", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusInternalServerError)
		})

		It("should map ALREADY_EXISTS mutation errors", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"CreateTeam": map[string]any{
					"createTeam": map[string]any{
						"team": nil,
						"errors": []map[string]any{{
							"code":    "ALREADY_EXISTS",
							"message": "Team already exists",
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"name":"dup","email":"dup@test.de","members":[]}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs/eni/teams", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusConflict)
		})

		It("should map BAD_REQUEST mutation errors", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"CreateTeam": map[string]any{
					"createTeam": map[string]any{
						"team": nil,
						"errors": []map[string]any{{
							"code":    "BAD_REQUEST",
							"message": "Invalid team name",
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"name":"bad!name","email":"x@test.de","members":[]}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs/eni/teams", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusBadRequest)
		})
	})

	Describe("UpdateTeam", func() {
		It("should return 400 for invalid JSON body", func() {
			gqlServer := mockGraphQLServer(nil)
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/hyperion", strings.NewReader("{bad"))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusBadRequest)
		})

		It("should return 404 when team not found", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": map[string]any{
					"teams": map[string]any{"edges": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"email":"new@test.de","members":[]}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/nonexistent", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNotFound)
		})

		It("should map PRECONDITION_FAILED mutation errors", func() {
			readyPhase := "READY"
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": map[string]any{
					"teams": map[string]any{
						"edges": []map[string]any{{
							"node": map[string]any{
								"id": "10", "name": "hyperion", "email": "h@test.de",
								"createdAt": now.Format(time.RFC3339), "lastModifiedAt": now.Format(time.RFC3339),
								"statusPhase": &readyPhase, "group": map[string]any{"name": "eni"}, "members": []any{},
							},
						}},
					},
				},
				"UpdateTeam": map[string]any{
					"updateTeam": map[string]any{
						"team": nil,
						"errors": []map[string]any{{
							"code":    "PRECONDITION_FAILED",
							"message": "Stale data",
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"email":"x@test.de","members":[]}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/hyperion", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusPreconditionFailed)
		})
	})

	Describe("DeleteTeam", func() {
		It("should return 404 when team not found", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": map[string]any{
					"teams": map[string]any{"edges": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodDelete, "/organization/v1/hubs/eni/teams/nonexistent", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNotFound)
		})
	})

	Describe("GetTeam - not found", func() {
		It("should return 404 when team not found", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": map[string]any{
					"teams": map[string]any{"edges": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/nonexistent", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNotFound)
		})
	})

	Describe("GetTeamStatus", func() {
		It("should return team status for READY phase", func() {
			readyPhase := "READY"
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": map[string]any{
					"teams": map[string]any{
						"edges": []map[string]any{{
							"node": map[string]any{
								"id": "10", "name": "hyperion", "email": "h@test.de",
								"createdAt":      now.Format(time.RFC3339),
								"lastModifiedAt": now.Format(time.RFC3339),
								"statusPhase":    &readyPhase,
								"group":          map[string]any{"name": "eni"},
								"members":        []any{},
							},
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion/status", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			result := expectJSON(resp, err)
			Expect(result["overallStatus"]).To(Equal("done"))
			Expect(result["processingState"]).To(Equal("done"))
			Expect(result["state"]).To(Equal("complete"))
		})

		It("should return team status for ERROR phase with message", func() {
			errorPhase := "ERROR"
			errMsg := "provisioning failed"
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": map[string]any{
					"teams": map[string]any{
						"edges": []map[string]any{{
							"node": map[string]any{
								"id": "10", "name": "hyperion", "email": "h@test.de",
								"createdAt":      now.Format(time.RFC3339),
								"lastModifiedAt": now.Format(time.RFC3339),
								"statusPhase":    &errorPhase,
								"statusMessage":  &errMsg,
								"group":          map[string]any{"name": "eni"},
								"members":        []any{},
							},
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion/status", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			result := expectJSON(resp, err)
			Expect(result["overallStatus"]).To(Equal("failed"))
			Expect(result["processingState"]).To(Equal("failed"))
			Expect(result["state"]).To(Equal("blocked"))
			errors := result["errors"].([]any)
			Expect(errors).To(HaveLen(1))
		})

		It("should return 404 when team not found", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": map[string]any{
					"teams": map[string]any{"edges": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/nonexistent/status", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNotFound)
		})
	})

	Describe("PatchTeamToken", func() {
		It("should return 404 when team not found", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": map[string]any{
					"teams": map[string]any{"edges": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodPatch, "/organization/v1/hubs/eni/teams/nonexistent/teamToken", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusNotFound)
		})

		It("should map VALIDATION_FAILED mutation errors", func() {
			readyPhase := "READY"
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": map[string]any{
					"teams": map[string]any{
						"edges": []map[string]any{{
							"node": map[string]any{
								"id": "10", "name": "hyperion", "email": "h@test.de",
								"createdAt": now.Format(time.RFC3339), "lastModifiedAt": now.Format(time.RFC3339),
								"statusPhase": &readyPhase, "group": map[string]any{"name": "eni"}, "members": []any{},
							},
						}},
					},
				},
				"RotateTeamToken": map[string]any{
					"rotateTeamToken": map[string]any{
						"team": nil,
						"errors": []map[string]any{{
							"code":    "VALIDATION_FAILED",
							"message": "Cannot rotate",
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodPatch, "/organization/v1/hubs/eni/teams/hyperion/teamToken", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusBadRequest)
		})
	})

	Describe("GetTeamResources", func() {
		It("should return 502 when rover-server is down", func() {
			readyPhase := "READY"
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": map[string]any{
					"teams": map[string]any{
						"edges": []map[string]any{{
							"node": map[string]any{
								"id": "10", "name": "hyperion", "email": "h@test.de",
								"createdAt": now.Format(time.RFC3339), "lastModifiedAt": now.Format(time.RFC3339),
								"statusPhase": &readyPhase, "group": map[string]any{"name": "eni"}, "members": []any{},
							},
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			// Point to a closed server
			app := newTestApp(gqlServer.URL, "http://127.0.0.1:1")

			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion/resources", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusInternalServerError)
		})

		It("should paginate resources", func() {
			roverResp := `{"items":[
				{"name":"r1","kind":"ApiExposure","apiVersion":"v1","path":"/r1"},
				{"name":"r2","kind":"ApiSubscription","apiVersion":"v1","path":"/r2"},
				{"name":"r3","kind":"EventExposure","apiVersion":"v1","path":"/r3"}
			]}`

			gqlServer := mockGraphQLServer(nil)
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(map[string]string{
				"/resources": roverResp,
			})
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams/hyperion/resources?offset=0&limit=2", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, _ := io.ReadAll(resp.Body)
			var result map[string]any
			Expect(json.Unmarshal(body, &result)).To(Succeed())

			items := result["items"].([]any)
			Expect(items).To(HaveLen(2))

			paging := result["paging"].(map[string]any)
			Expect(paging["total"]).To(BeNumerically("==", 3))
		})
	})

	Describe("ListTeams with pagination", func() {
		It("should handle offset beyond total", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"ListTeams": map[string]any{
					"teams": map[string]any{
						"edges": []map[string]any{{
							"node": map[string]any{
								"id": "10", "name": "hyperion", "email": "h@test.de",
								"createdAt":      now.Format(time.RFC3339),
								"lastModifiedAt": now.Format(time.RFC3339),
								"group":          map[string]any{"name": "eni"},
								"members":        []any{},
							},
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs/eni/teams?offset=100&limit=10", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, _ := io.ReadAll(resp.Body)
			var result map[string]any
			Expect(json.Unmarshal(body, &result)).To(Succeed())

			items := result["items"].([]any)
			Expect(items).To(BeEmpty())
		})
	})

	Describe("Mutation error code mapping", func() {
		It("should map unknown error codes to 500", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"CreateTeam": map[string]any{
					"createTeam": map[string]any{
						"team": nil,
						"errors": []map[string]any{{
							"code":    "UNEXPECTED_ERROR",
							"message": "Something weird happened",
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"name":"team","email":"t@test.de","members":[]}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs/eni/teams", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusInternalServerError)

			respBody, _ := io.ReadAll(resp.Body)
			var errResp map[string]any
			Expect(json.Unmarshal(respBody, &errResp)).To(Succeed())
			Expect(errResp["title"]).To(Equal("UNEXPECTED_ERROR"))
			Expect(errResp["detail"]).To(Equal("Something weird happened"))
		})
	})
})

// Ensure unused import doesn't cause issues
var _ = Describe("Middleware integration", func() {
	It("should reject malformed token", func() {
		gqlServer := mockGraphQLServer(nil)
		DeferCleanup(gqlServer.Close)
		roverServer := mockRoverServer(nil)
		DeferCleanup(roverServer.Close)
		app := newTestApp(gqlServer.URL, roverServer.URL)

		req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs", http.NoBody)
		req.Header.Set("Authorization", "Bearer not-a-valid-jwt")
		resp, err := app.Test(req, -1)
		expectStatus(resp, err, http.StatusUnauthorized)
	})

	It("should reject token missing clientId", func() {
		gqlServer := mockGraphQLServer(nil)
		DeferCleanup(gqlServer.Close)
		roverServer := mockRoverServer(nil)
		DeferCleanup(roverServer.Close)
		app := newTestApp(gqlServer.URL, roverServer.URL)

		// Token with only exp claim, no clientId
		tokenWithoutClientId := makeTokenWithClaims(map[string]any{
			"exp":   time.Now().Add(time.Hour).Unix(),
			"scope": "openid",
		})
		req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs", http.NoBody)
		resp, err := executeRequest(app, req, tokenWithoutClientId)
		expectStatus(resp, err, http.StatusUnauthorized)
	})

	It("should reject token with invalid clientId format", func() {
		gqlServer := mockGraphQLServer(nil)
		DeferCleanup(gqlServer.Close)
		roverServer := mockRoverServer(nil)
		DeferCleanup(roverServer.Close)
		app := newTestApp(gqlServer.URL, roverServer.URL)

		tokenBadClientId := makeTokenWithClaims(map[string]any{
			"exp":      time.Now().Add(time.Hour).Unix(),
			"clientId": "no-dashes-here",
		})
		req := httptest.NewRequest(http.MethodGet, "/organization/v1/hubs", http.NoBody)
		resp, err := executeRequest(app, req, tokenBadClientId)
		expectStatus(resp, err, http.StatusUnauthorized)
	})
})
