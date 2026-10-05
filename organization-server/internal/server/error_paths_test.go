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
			expectProblem(resp, err, http.StatusBadRequest, "Bad Request", "Invalid request body")
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
			expectProblem(resp, err, http.StatusInternalServerError, "Internal Server Error", "Unable to create hub")
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

		It("should return 500 when the CP API rejects an async mutation without a payload", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"CreateGroup": map[string]any{
					"createGroup": map[string]any{"group": nil, "accepted": false, "errors": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"name":"new-hub","displayName":"New Hub","description":"desc"}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectProblem(resp, err, http.StatusInternalServerError, "Internal Server Error", "Unable to create hub")
		})

		It("should return 202 with request data when the CP API returns no group", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"CreateGroup": map[string]any{
					"createGroup": map[string]any{"group": nil, "accepted": true, "errors": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"name":"new-hub","displayName":"New Hub","description":"desc"}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))
			var result map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result["name"]).To(Equal("new-hub"))
			Expect(result["displayName"]).To(Equal("New Hub"))
			Expect(result["description"]).To(Equal("desc"))
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

		It("should create the hub when it does not exist (upsert)", func() {
			var ops []string
			var createVars map[string]any
			gqlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req struct {
					OperationName string         `json:"operationName"`
					Variables     map[string]any `json:"variables"`
				}
				_ = json.Unmarshal(body, &req)
				ops = append(ops, req.OperationName)

				var data map[string]any
				switch req.OperationName {
				case "GetGroup":
					data = map[string]any{"groups": []any{}}
				case "CreateGroup":
					createVars, _ = req.Variables["input"].(map[string]any)
					data = map[string]any{"createGroup": map[string]any{
						"group": map[string]any{
							"id": "9", "name": "nonexistent", "displayName": "Updated", "description": "updated",
						},
						"errors": []any{},
					}}
				}
				w.Header().Set("Content-Type", "application/json")
				out, _ := json.Marshal(map[string]any{"data": data})
				_, _ = w.Write(out)
			}))
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"displayName":"Updated","description":"updated"}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/nonexistent", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			result := expectJSONStatus(resp, err, http.StatusAccepted)
			Expect(result["name"]).To(Equal("nonexistent"))
			Expect(ops).To(Equal([]string{"GetGroup", "CreateGroup"}))
			Expect(createVars).To(HaveKeyWithValue("name", "nonexistent"))
			Expect(createVars).To(HaveKeyWithValue("displayName", "Updated"))
			Expect(createVars).To(HaveKeyWithValue("description", "updated"))
		})

		It("should map mutation errors when creating a missing hub", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetGroup": map[string]any{"groups": []any{}},
				"CreateGroup": map[string]any{
					"createGroup": map[string]any{
						"group": nil,
						"errors": []map[string]any{{
							"code":    "FORBIDDEN",
							"message": "only admins can create groups",
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"displayName":"Updated","description":"updated"}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/nonexistent", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusForbidden)
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
			expectProblem(resp, err, http.StatusNotFound, "Not Found", "Hub not found: nonexistent")
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
				"GetGroup": existingHubResponse(),
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
				"GetGroup": existingHubResponse(),
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

		It("should return 404 without creating the team when hub does not exist", func() {
			var ops []string
			gqlServer := mockGraphQLServer(map[string]any{
				"GetGroup": map[string]any{"groups": []any{}},
			}, &ops)
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"name":"newteam","email":"t@test.de","members":[]}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs/missing/teams", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectProblem(resp, err, http.StatusNotFound, "NOT_FOUND", "Hub not found: missing")
			Expect(ops).NotTo(ContainElement("CreateTeam"))
		})

		It("should return 500 when the CP API rejects an async team mutation without a payload", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetGroup": existingHubResponse(),
				"CreateTeam": map[string]any{
					"createTeam": map[string]any{"team": nil, "accepted": false, "errors": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"name":"newteam","email":"t@test.de","members":[{"name":"Alice","email":"alice@test.de"}]}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs/eni/teams", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectProblem(resp, err, http.StatusInternalServerError, "Internal Server Error", "Unable to create team")
		})

		It("should return 202 with request data when the CP API returns no team", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetGroup": existingHubResponse(),
				"CreateTeam": map[string]any{
					"createTeam": map[string]any{"team": nil, "accepted": true, "errors": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"name":"newteam","email":"t@test.de","members":[{"name":"Alice","email":"alice@test.de"}]}`
			req := httptest.NewRequest(http.MethodPost, "/organization/v1/hubs/eni/teams", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))
			var result map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result["name"]).To(Equal("newteam"))
			Expect(result["email"]).To(Equal("t@test.de"))
			Expect(result["clientId"]).To(Equal("eni--newteam--team-user"))
			Expect(result["members"]).To(HaveLen(1))
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

		It("should create the team when it does not exist (upsert)", func() {
			var ops []string
			var createVars map[string]any
			gqlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req struct {
					OperationName string         `json:"operationName"`
					Variables     map[string]any `json:"variables"`
				}
				_ = json.Unmarshal(body, &req)
				ops = append(ops, req.OperationName)

				var data map[string]any
				switch req.OperationName {
				case "GetTeam":
					data = map[string]any{"teams": map[string]any{"edges": []any{}}}
				case "CreateTeam":
					createVars, _ = req.Variables["input"].(map[string]any)
					data = map[string]any{"createTeam": map[string]any{
						"team": map[string]any{
							"id": "11", "name": "nonexistent", "email": "new@test.de",
							"createdAt": now.Format(time.RFC3339), "lastModifiedAt": now.Format(time.RFC3339),
							"statusPhase": nil, "group": map[string]any{"name": "eni"}, "members": []any{},
						},
						"errors": []any{},
					}}
				}
				w.Header().Set("Content-Type", "application/json")
				out, _ := json.Marshal(map[string]any{"data": data})
				_, _ = w.Write(out)
			}))
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"email":"new@test.de","members":[{"name":"Jane","email":"jane@test.de"}]}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/nonexistent", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			result := expectJSONStatus(resp, err, http.StatusAccepted)
			Expect(result["name"]).To(Equal("nonexistent"))
			Expect(ops).To(Equal([]string{"GetTeam", "CreateTeam"}))
			Expect(createVars).To(HaveKeyWithValue("group", "eni"))
			Expect(createVars).To(HaveKeyWithValue("name", "nonexistent"))
			Expect(createVars).To(HaveKeyWithValue("email", "new@test.de"))
			Expect(createVars["members"]).To(HaveLen(1))
		})

		It("should map mutation errors when creating a missing team", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": map[string]any{
					"teams": map[string]any{"edges": []any{}},
				},
				"CreateTeam": map[string]any{
					"createTeam": map[string]any{
						"team": nil,
						"errors": []map[string]any{{
							"code":    "FORBIDDEN",
							"message": "insufficient permissions",
						}},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"email":"new@test.de","members":[]}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/nonexistent", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			expectStatus(resp, err, http.StatusForbidden)
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

		It("should return 500 when the CP API rejects a token rotation without a payload", func() {
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
						"team":     nil,
						"accepted": false,
						"errors":   []any{},
					},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodPatch, "/organization/v1/hubs/eni/teams/hyperion/teamToken", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			expectProblem(resp, err, http.StatusInternalServerError, "Internal Server Error", "Unable to rotate team token")
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

	Describe("Async mutation payloads without returned resources", func() {
		teamResponse := func() map[string]any {
			return map[string]any{
				"teams": map[string]any{
					"edges": []map[string]any{{
						"node": map[string]any{
							"id": "10", "name": "eni--hyperion", "email": "h@test.de",
							"createdAt": now.Format(time.RFC3339), "lastModifiedAt": now.Format(time.RFC3339),
							"group": map[string]any{"name": "eni"}, "members": []any{},
						},
					}},
				},
			}
		}

		It("should return 202 for UpdateHub when the CP API returns no group", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetGroup": existingHubResponse(),
				"UpdateGroup": map[string]any{
					"updateGroup": map[string]any{"group": nil, "accepted": true, "errors": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"displayName":"Updated","description":"updated"}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))
			var result map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result["name"]).To(Equal("eni"))
			Expect(result["displayName"]).To(Equal("Updated"))
		})

		It("should return 202 for UpdateTeam when the CP API returns no team", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": teamResponse(),
				"UpdateTeam": map[string]any{
					"updateTeam": map[string]any{"team": nil, "accepted": true, "errors": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			body := `{"email":"new@test.de","members":[]}`
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/hyperion", strings.NewReader(body))
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))
			var result map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result["name"]).To(Equal("hyperion"))
			Expect(result["email"]).To(Equal("new@test.de"))
			Expect(result["members"]).To(Equal([]any{}))
		})

		It("should return 200 with an empty token when the accepted rotation has no team payload", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetTeam": teamResponse(),
				"RotateTeamToken": map[string]any{
					"rotateTeamToken": map[string]any{"team": nil, "accepted": true, "errors": []any{}},
				},
			})
			DeferCleanup(gqlServer.Close)
			roverServer := mockRoverServer(nil)
			DeferCleanup(roverServer.Close)
			app := newTestApp(gqlServer.URL, roverServer.URL)

			req := httptest.NewRequest(http.MethodPatch, "/organization/v1/hubs/eni/teams/hyperion/teamToken", http.NoBody)
			resp, err := executeRequest(app, req, adminToken)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			var result map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result["teamToken"]).To(Equal(""))
		})
	})

	Describe("Mutation error code mapping", func() {
		It("should map unknown error codes to 500", func() {
			gqlServer := mockGraphQLServer(map[string]any{
				"GetGroup": existingHubResponse(),
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
			expectProblem(resp, err, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Something weird happened")
		})
	})
})

func existingHubResponse() map[string]any {
	return map[string]any{
		"groups": []map[string]any{{
			"id": "1", "name": "eni", "displayName": "Eni", "description": "",
		}},
	}
}

func expectProblem(resp *http.Response, err error, status int, title, detail string) {
	GinkgoHelper()
	Expect(err).ToNot(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(status))
	Expect(resp.Header.Get("Content-Type")).To(HavePrefix("application/problem+json"))

	body, readErr := io.ReadAll(resp.Body)
	Expect(readErr).ToNot(HaveOccurred())
	var problem map[string]any
	Expect(json.Unmarshal(body, &problem)).To(Succeed())
	Expect(problem["status"]).To(BeNumerically("==", status))
	Expect(problem["title"]).To(Equal(title))
	Expect(problem["detail"]).To(Equal(detail))
}

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
		expectStatus(resp, err, http.StatusForbidden)
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
		expectStatus(resp, err, http.StatusForbidden)
	})
})
