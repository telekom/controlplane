// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/Khan/genqlient/graphql"

	"github.com/telekom/controlplane/organization-server/internal/api"
	"github.com/telekom/controlplane/organization-server/internal/controller"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpdateTeam membership regression", func() {
	DescribeTable("forwards a requested member addition for an existing team", func(direct bool) {
		type operation struct {
			Name      string          `json:"operationName"`
			Variables json.RawMessage `json:"variables"`
		}
		var mu sync.Mutex
		var operations []operation
		gqlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			var op operation
			Expect(json.NewDecoder(r.Body).Decode(&op)).To(Succeed())
			mu.Lock()
			operations = append(operations, op)
			mu.Unlock()

			var data any
			switch op.Name {
			case "GetTeam":
				data = map[string]any{"teams": map[string]any{"edges": []any{
					map[string]any{"node": map[string]any{
						"id": "10", "name": "eni--hyperion", "email": "team@example.com",
						"createdAt": "2026-01-01T00:00:00Z", "lastModifiedAt": "2026-01-01T00:00:00Z",
						"group": map[string]any{"name": "eni"}, "members": []any{},
					}},
				}}}
			case "UpdateTeam":
				data = map[string]any{"updateTeam": map[string]any{
					"accepted": true, "team": nil, "errors": []any{},
				}}
			default:
				Fail("unexpected GraphQL operation: " + op.Name)
			}
			w.Header().Set("Content-Type", "application/json")
			Expect(json.NewEncoder(w).Encode(map[string]any{"data": data})).To(Succeed())
		}))
		DeferCleanup(gqlServer.Close)
		body := `{"displayName":"Hyperion","description":"","email":"team@example.com","members":[{"name":"Bob","email":"bob@example.com"}]}`
		if direct {
			ctrl := controller.New(graphql.NewClient(gqlServer.URL, gqlServer.Client()), nil)
			var input *api.TeamUpdateRequest
			Expect(json.Unmarshal([]byte(body), &input)).To(Succeed())
			Expect(*input.Members).To(HaveLen(1))
			result, mutationErrors, err := ctrl.UpdateTeam(context.Background(), "test", "eni", "hyperion", input)
			Expect(err).NotTo(HaveOccurred())
			Expect(mutationErrors).To(BeEmpty())
			Expect(result).NotTo(BeNil())
		} else {
			app := newTestApp(gqlServer.URL, "http://unused.invalid")
			req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/hyperion", strings.NewReader(body))
			resp, err := executeRequest(app, req, makeToken("eni", "hyperion", []string{"tardis:admin:all"}))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(resp.Body.Close()).To(Succeed()) })
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))
		}

		mu.Lock()
		captured := append([]operation(nil), operations...)
		mu.Unlock()
		Expect(captured).To(HaveLen(2))
		Expect(captured[1].Name).To(Equal("UpdateTeam"))
		var variables struct {
			Input struct {
				TeamID  string           `json:"teamId"`
				Members []api.TeamMember `json:"members"`
			} `json:"input"`
		}
		Expect(json.Unmarshal(captured[1].Variables, &variables)).To(Succeed())
		Expect(variables.Input.TeamID).To(Equal("10"))
		Expect(variables.Input.Members).To(Equal([]api.TeamMember{{Name: "Bob", Email: "bob@example.com"}}),
			"HTTP 202 must be backed by an upstream mutation carrying the requested member addition")
	}, Entry("through REST", false), Entry("directly at controller entry", true))

	DescribeTable("sends the complete desired list independently of the projection", func(members []api.TeamMember, withProjection bool) {
		captured := make(chan map[string]any, 1)
		projected := map[string]any{
			"id": "10", "name": "eni--hyperion", "email": "team@example.com",
			"createdAt": "2026-01-01T00:00:00Z", "lastModifiedAt": "2026-01-01T00:00:00Z",
			"members": []api.TeamMember{{Name: "Alice", Email: "alice@example.com"}},
		}
		gqlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			var op struct {
				Name      string `json:"operationName"`
				Variables struct {
					Input map[string]any `json:"input"`
				} `json:"variables"`
			}
			Expect(json.NewDecoder(r.Body).Decode(&op)).To(Succeed())
			var data any
			switch op.Name {
			case "GetTeam":
				data = map[string]any{"teams": map[string]any{"edges": []any{map[string]any{"node": projected}}}}
			case "UpdateTeam":
				captured <- op.Variables.Input
				var team any
				if withProjection {
					team = projected
				}
				data = map[string]any{"updateTeam": map[string]any{"accepted": true, "team": team, "errors": []any{}}}
			default:
				Fail("unexpected operation: " + op.Name)
			}
			Expect(json.NewEncoder(w).Encode(map[string]any{"data": data})).To(Succeed())
		}))
		DeferCleanup(gqlServer.Close)
		app := newTestApp(gqlServer.URL, "http://unused.invalid")
		body, err := json.Marshal(api.TeamUpdateRequest{Email: ptr("team@example.com"), DisplayName: ptr("Hyperion"), Members: &members})
		Expect(err).NotTo(HaveOccurred())
		req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/hyperion", strings.NewReader(string(body)))
		resp, err := executeRequest(app, req, makeToken("eni", "hyperion", []string{"tardis:admin:all"}))
		result := expectJSONStatus(resp, err, http.StatusAccepted)
		DeferCleanup(func() { Expect(resp.Body.Close()).To(Succeed()) })
		var input map[string]any
		Expect(captured).To(Receive(&input))
		serialized, err := json.Marshal(input["members"])
		Expect(err).NotTo(HaveOccurred())
		expected, err := json.Marshal(members)
		Expect(err).NotTo(HaveOccurred())
		Expect(serialized).To(MatchJSON(expected))
		Expect(result["status"]).To(HaveKeyWithValue("processingState", "pending"))
		if withProjection {
			Expect(result["members"]).To(ConsistOf(map[string]any{"name": "Alice", "email": "alice@example.com"}))
		} else {
			responseMembers, err := json.Marshal(result["members"])
			Expect(err).NotTo(HaveOccurred())
			Expect(responseMembers).To(MatchJSON(expected))
		}
	},
		Entry("remove old and add new with stale response", []api.TeamMember{{Name: "Bob", Email: "bob@example.com"}}, true),
		Entry("explicit empty array reaches upstream validation", []api.TeamMember{}, false),
		Entry("unchanged", []api.TeamMember{{Name: "Alice", Email: "alice@example.com"}}, true),
		Entry("rename", []api.TeamMember{{Name: "Alice Updated", Email: "alice@example.com"}}, false),
	)

	DescribeTable("does not accept failed membership updates", func(response any, status int) {
		gqlServer := mockGraphQLServer(map[string]any{
			"GetTeam":    map[string]any{"teams": map[string]any{"edges": []any{map[string]any{"node": map[string]any{"id": "10"}}}}},
			"UpdateTeam": response,
		})
		DeferCleanup(gqlServer.Close)
		app := newTestApp(gqlServer.URL, "http://unused.invalid")
		req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/hyperion", strings.NewReader(`{"email":"team@example.com","members":[]}`))
		resp, err := executeRequest(app, req, makeToken("eni", "hyperion", []string{"tardis:admin:all"}))
		expectStatus(resp, err, status)
		DeferCleanup(func() { Expect(resp.Body.Close()).To(Succeed()) })
	},
		Entry("validation", map[string]any{"updateTeam": map[string]any{"errors": []any{map[string]any{"code": "VALIDATION_FAILED", "message": "invalid members"}}}}, http.StatusBadRequest),
		Entry("conflict", map[string]any{"updateTeam": map[string]any{"errors": []any{map[string]any{"code": "CONFLICT", "message": "conflict"}}}}, http.StatusConflict),
		Entry("not accepted", map[string]any{"updateTeam": map[string]any{"accepted": false, "team": nil, "errors": []any{}}}, http.StatusInternalServerError),
	)

	It("returns a problem when the upstream transport fails", func() {
		gqlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			var op struct {
				Name string `json:"operationName"`
			}
			Expect(json.NewDecoder(r.Body).Decode(&op)).To(Succeed())
			if op.Name == "GetTeam" {
				Expect(json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"teams": map[string]any{"edges": []any{map[string]any{"node": map[string]any{"id": "10"}}}}}})).To(Succeed())
				return
			}
			http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
		}))
		DeferCleanup(gqlServer.Close)
		app := newTestApp(gqlServer.URL, "http://unused.invalid")
		req := httptest.NewRequest(http.MethodPut, "/organization/v1/hubs/eni/teams/hyperion", strings.NewReader(`{"email":"team@example.com","members":[{"name":"Bob","email":"bob@example.com"}]}`))
		resp, err := executeRequest(app, req, makeToken("eni", "hyperion", []string{"tardis:admin:all"}))
		expectStatus(resp, err, http.StatusInternalServerError)
		DeferCleanup(func() { Expect(resp.Body.Close()).To(Succeed()) })
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))
	})
})

func ptr[T any](v T) *T { return &v }
