// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// capturingTeamServer answers team GraphQL operations and records the mutation input per operation.
func capturingTeamServer(existing, mutationTeam map[string]any) (*httptest.Server, map[string]json.RawMessage) {
	captured := map[string]json.RawMessage{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer GinkgoRecover()
		var op struct {
			Name      string `json:"operationName"`
			Variables struct {
				Input json.RawMessage `json:"input"`
			} `json:"variables"`
		}
		Expect(json.NewDecoder(r.Body).Decode(&op)).To(Succeed())
		var data any
		edges := []any{}
		if existing != nil {
			edges = append(edges, map[string]any{"node": existing})
		}
		switch op.Name {
		case "GetTeam", "ListTeams":
			data = map[string]any{"teams": map[string]any{"edges": edges}}
		case "GetGroup":
			data = map[string]any{"groups": []any{map[string]any{"id": "1", "name": "eni"}}}
		case "UpdateTeam":
			captured[op.Name] = op.Variables.Input
			data = map[string]any{"updateTeam": map[string]any{"accepted": true, "team": mutationTeam, "errors": []any{}}}
		case "CreateTeam":
			captured[op.Name] = op.Variables.Input
			data = map[string]any{"createTeam": map[string]any{"accepted": true, "team": mutationTeam, "errors": []any{}}}
		default:
			Fail("unexpected operation: " + op.Name)
		}
		w.Header().Set("Content-Type", "application/json")
		Expect(json.NewEncoder(w).Encode(map[string]any{"data": data})).To(Succeed())
	}))
	return srv, captured
}

func projectedTeam() map[string]any {
	return map[string]any{
		"id": "10", "name": "eni--hyperion", "email": "team@example.com",
		"displayName": "Hyperion", "description": "Projected description",
		"createdAt": "2026-01-01T00:00:00Z", "lastModifiedAt": "2026-01-01T00:00:00Z",
		"members": []any{map[string]any{"name": "Alice", "email": "alice@example.com"}},
	}
}

func sendTeamRequest(gqlURL, method, path, body string) (map[string]any, int) {
	app := newTestApp(gqlURL, "http://unused.invalid")
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	resp, err := executeRequest(app, req, makeToken("eni", "hyperion", []string{"tardis:admin:all"}))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(resp.Body.Close()).To(Succeed()) })
	var result map[string]any
	if resp.StatusCode != http.StatusNoContent {
		Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
	}
	return result, resp.StatusCode
}

func decodeInput(raw json.RawMessage) map[string]any {
	Expect(raw).NotTo(BeNil(), "mutation was not sent")
	var input map[string]any
	Expect(json.Unmarshal(raw, &input)).To(Succeed())
	return input
}

var _ = Describe("Team metadata and presence-aware upsert", func() {
	const teamPath = "/organization/v1/hubs/eni/teams/hyperion"

	DescribeTable("PUT on an existing team forwards only provided fields",
		func(body string, expected map[string]any) {
			srv, captured := capturingTeamServer(projectedTeam(), nil)
			DeferCleanup(srv.Close)
			_, status := sendTeamRequest(srv.URL, http.MethodPut, teamPath, body)
			Expect(status).To(Equal(http.StatusAccepted))
			input := decodeInput(captured["UpdateTeam"])
			Expect(input).To(HaveKeyWithValue("teamId", "10"))
			for _, field := range []string{"email", "members", "displayName", "description"} {
				if value, ok := expected[field]; ok {
					Expect(input).To(HaveKeyWithValue(field, value), field)
				} else {
					Expect(input[field]).To(BeNil(), field+" must not be sent as a zero value")
				}
			}
		},
		Entry("metadata only", `{"displayName":"New name"}`, map[string]any{"displayName": "New name"}),
		Entry("description only", `{"description":"New description"}`, map[string]any{"description": "New description"}),
		Entry("empty description clears", `{"description":""}`, map[string]any{"description": ""}),
		Entry("explicit nulls preserve", `{"email":null,"members":null,"displayName":null,"description":null}`, map[string]any{}),
		Entry("empty object preserves", `{}`, map[string]any{}),
		Entry("explicit empty members is forwarded for rejection", `{"members":[]}`, map[string]any{"members": []any{}}),
		Entry("all fields", `{"email":"new@example.com","displayName":"New","description":"Desc","members":[{"name":"Bob","email":"bob@example.com"}]}`,
			map[string]any{
				"email": "new@example.com", "displayName": "New", "description": "Desc",
				"members": []any{map[string]any{"name": "Bob", "email": "bob@example.com"}},
			}),
	)

	It("returns supplied metadata in the accepted response without projection", func() {
		srv, _ := capturingTeamServer(projectedTeam(), nil)
		DeferCleanup(srv.Close)
		result, status := sendTeamRequest(srv.URL, http.MethodPut, teamPath, `{"displayName":"New","description":"Desc"}`)
		Expect(status).To(Equal(http.StatusAccepted))
		Expect(result).To(HaveKeyWithValue("displayName", "New"))
		Expect(result).To(HaveKeyWithValue("description", "Desc"))
		Expect(result["status"]).To(HaveKeyWithValue("processingState", "pending"))
	})

	It("maps metadata from a returned (possibly stale) projection", func() {
		srv, _ := capturingTeamServer(projectedTeam(), projectedTeam())
		DeferCleanup(srv.Close)
		result, status := sendTeamRequest(srv.URL, http.MethodPut, teamPath, `{"displayName":"New"}`)
		Expect(status).To(Equal(http.StatusAccepted))
		Expect(result).To(HaveKeyWithValue("displayName", "Hyperion"))
		Expect(result).To(HaveKeyWithValue("description", "Projected description"))
	})

	Describe("PUT on a missing team", func() {
		It("creates the team forwarding provided metadata", func() {
			srv, captured := capturingTeamServer(nil, nil)
			DeferCleanup(srv.Close)
			result, status := sendTeamRequest(srv.URL, http.MethodPut, teamPath,
				`{"email":"team@example.com","displayName":"Hyperion","description":"","members":[{"name":"Bob","email":"bob@example.com"}]}`)
			Expect(status).To(Equal(http.StatusAccepted))
			input := decodeInput(captured["CreateTeam"])
			Expect(input).To(HaveKeyWithValue("name", "hyperion"))
			Expect(input).To(HaveKeyWithValue("displayName", "Hyperion"))
			Expect(input).To(HaveKeyWithValue("description", ""))
			Expect(result).To(HaveKeyWithValue("displayName", "Hyperion"))
		})

		It("leaves omitted metadata unset", func() {
			srv, captured := capturingTeamServer(nil, nil)
			DeferCleanup(srv.Close)
			_, status := sendTeamRequest(srv.URL, http.MethodPut, teamPath,
				`{"email":"team@example.com","members":[{"name":"Bob","email":"bob@example.com"}]}`)
			Expect(status).To(Equal(http.StatusAccepted))
			input := decodeInput(captured["CreateTeam"])
			Expect(input["displayName"]).To(BeNil())
			Expect(input["description"]).To(BeNil())
		})

		DescribeTable("rejects a create without required data and sends no mutation", func(body string) {
			srv, captured := capturingTeamServer(nil, nil)
			DeferCleanup(srv.Close)
			result, status := sendTeamRequest(srv.URL, http.MethodPut, teamPath, body)
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(result["detail"]).To(ContainSubstring("email and at least one member are required"))
			Expect(captured).To(BeEmpty())
		},
			Entry("metadata only", `{"displayName":"Hyperion"}`),
			Entry("missing members", `{"email":"team@example.com"}`),
			Entry("empty members", `{"email":"team@example.com","members":[]}`),
			Entry("missing email", `{"members":[{"name":"Bob","email":"bob@example.com"}]}`),
		)
	})

	DescribeTable("POST forwards metadata", func(body string, expected map[string]any) {
		srv, captured := capturingTeamServer(nil, nil)
		DeferCleanup(srv.Close)
		result, status := sendTeamRequest(srv.URL, http.MethodPost, "/organization/v1/hubs/eni/teams", body)
		Expect(status).To(Equal(http.StatusAccepted))
		input := decodeInput(captured["CreateTeam"])
		for field, value := range expected {
			if value == nil {
				Expect(input[field]).To(BeNil(), field)
				Expect(result).To(HaveKeyWithValue(field, ""))
				continue
			}
			Expect(input).To(HaveKeyWithValue(field, value))
			Expect(result).To(HaveKeyWithValue(field, value))
		}
	},
		Entry("both", `{"name":"hyperion","email":"team@example.com","displayName":"Hyperion","description":"Desc","members":[{"name":"Bob","email":"bob@example.com"}]}`,
			map[string]any{"displayName": "Hyperion", "description": "Desc"}),
		Entry("omitted", `{"name":"hyperion","email":"team@example.com","members":[{"name":"Bob","email":"bob@example.com"}]}`,
			map[string]any{"displayName": nil, "description": nil}),
	)

	DescribeTable("POST on an existing team forwards the description pointer unchanged",
		func(description string, expected any) {
			srv, captured := capturingTeamServer(projectedTeam(), nil)
			DeferCleanup(srv.Close)
			body := `{"name":"hyperion","email":"team@example.com","displayName":"Hyperion",` + description +
				`"members":[{"name":"Bob","email":"bob@example.com"}]}`
			result, status := sendTeamRequest(srv.URL, http.MethodPost, "/organization/v1/hubs/eni/teams", body)
			Expect(status).To(Equal(http.StatusAccepted))
			input := decodeInput(captured["CreateTeam"])
			if expected == nil {
				Expect(input["description"]).To(BeNil())
				return
			}
			Expect(input).To(HaveKeyWithValue("description", expected))
			Expect(result).To(HaveKeyWithValue("description", expected))
		},
		Entry("omitted preserves", ``, nil),
		Entry("null preserves", `"description":null,`, nil),
		Entry("empty string clears", `"description":"",`, ""),
		Entry("provided", `"description":"Desc",`, "Desc"),
	)

	It("maps metadata from the create mutation response", func() {
		srv, _ := capturingTeamServer(nil, projectedTeam())
		DeferCleanup(srv.Close)
		result, status := sendTeamRequest(srv.URL, http.MethodPost, "/organization/v1/hubs/eni/teams",
			`{"name":"hyperion","email":"team@example.com","members":[{"name":"Alice","email":"alice@example.com"}]}`)
		Expect(status).To(Equal(http.StatusAccepted))
		Expect(result).To(HaveKeyWithValue("displayName", "Hyperion"))
		Expect(result).To(HaveKeyWithValue("description", "Projected description"))
	})

	It("maps metadata on get and list", func() {
		srv, _ := capturingTeamServer(projectedTeam(), nil)
		DeferCleanup(srv.Close)
		result, status := sendTeamRequest(srv.URL, http.MethodGet, teamPath, "")
		Expect(status).To(Equal(http.StatusOK))
		Expect(result).To(HaveKeyWithValue("displayName", "Hyperion"))
		Expect(result).To(HaveKeyWithValue("description", "Projected description"))

		list, status := sendTeamRequest(srv.URL, http.MethodGet, "/organization/v1/hubs/eni/teams", "")
		Expect(status).To(Equal(http.StatusOK))
		Expect(list["items"]).To(ConsistOf(SatisfyAll(
			HaveKeyWithValue("displayName", "Hyperion"),
			HaveKeyWithValue("description", "Projected description"),
		)))
	})
})
