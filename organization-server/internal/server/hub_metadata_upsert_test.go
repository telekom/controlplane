// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func capturingGroupServer(existing bool) (*httptest.Server, map[string]json.RawMessage) {
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
		groups := []any{}
		if existing {
			groups = append(groups, map[string]any{"id": "1", "name": "eni", "displayName": "Eni", "description": "Stored"})
		}
		var data any
		switch op.Name {
		case "GetGroup":
			data = map[string]any{"groups": groups}
		case "CreateGroup":
			captured[op.Name] = op.Variables.Input
			data = map[string]any{"createGroup": map[string]any{"accepted": true, "group": nil, "errors": []any{}}}
		case "UpdateGroup":
			captured[op.Name] = op.Variables.Input
			data = map[string]any{"updateGroup": map[string]any{"accepted": true, "group": nil, "errors": []any{}}}
		default:
			Fail("unexpected operation: " + op.Name)
		}
		w.Header().Set("Content-Type", "application/json")
		Expect(json.NewEncoder(w).Encode(map[string]any{"data": data})).To(Succeed())
	}))
	return srv, captured
}

var _ = Describe("Hub description presence and deprecated email", func() {
	type hubCase struct {
		existing  bool
		method    string
		path      string
		operation string
	}
	cases := map[string]hubCase{
		"PUT on an existing hub": {existing: true, method: http.MethodPut, path: "/organization/v1/hubs/eni", operation: "UpdateGroup"},
		"PUT on a missing hub":   {existing: false, method: http.MethodPut, path: "/organization/v1/hubs/eni", operation: "CreateGroup"},
		"POST":                   {existing: false, method: http.MethodPost, path: "/organization/v1/hubs", operation: "CreateGroup"},
	}

	for name, tc := range cases {
		DescribeTable(name+" forwards the description only when provided",
			func(fields string, expected *string) {
				srv, captured := capturingGroupServer(tc.existing)
				DeferCleanup(srv.Close)
				body := `{"name":"eni","displayName":"Eni"` + fields + `}`
				result, status := sendTeamRequest(srv.URL, tc.method, tc.path, body)
				Expect(status).To(Equal(http.StatusAccepted))
				input := decodeInput(captured[tc.operation])
				Expect(input).To(HaveKeyWithValue("displayName", "Eni"))
				Expect(input).NotTo(HaveKey("email"))
				Expect(result).NotTo(HaveKey("email"))
				if expected == nil {
					Expect(input["description"]).To(BeNil(), "description must not be sent as a zero value")
					Expect(result).NotTo(HaveKey("description"))
					return
				}
				Expect(input).To(HaveKeyWithValue("description", *expected))
				if *expected != "" {
					Expect(result).To(HaveKeyWithValue("description", *expected))
				}
			},
			Entry("omitted preserves", ``, nil),
			Entry("explicit null preserves", `,"description":null`, nil),
			Entry("empty clears", `,"description":""`, hubDescriptionPtr("")),
			Entry("value updates", `,"description":"New description"`, hubDescriptionPtr("New description")),
			Entry("deprecated email is ignored", `,"email":"hub@example.com","description":"Desc"`, hubDescriptionPtr("Desc")),
		)
	}
})

func hubDescriptionPtr(s string) *string {
	return &s
}
