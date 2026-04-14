// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"net/http"
	"net/http/httptest"

	"github.com/gkampitakis/go-snaps/match"
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("EventType Controller", func() {

	Describe("GET /eventtypes", func() {
		It("should list event types with team token", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventtypes", nil)
			resp, err := ExecuteRequest(req, teamToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body, match.Any("items.0.status"))
		})

		It("should list event types with admin token", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventtypes", nil)
			resp, err := ExecuteRequest(req, adminToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body, match.Any("items.0.status"))
		})
	})

	Describe("GET /eventtypes/:eventTypeId", func() {
		It("should return a single event type", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventtypes/eni--hyperion--de-telekom-eni-quickstart-v1", nil)
			resp, err := ExecuteRequest(req, teamToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body, match.Any("status"))
		})

	})

	Describe("GET /eventtypes/:eventTypeId/status", func() {
		It("should return event type status", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventtypes/eni--hyperion--de-telekom-eni-quickstart-v1/status", nil)
			resp, err := ExecuteRequest(req, teamToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body, match.Any("createdAt"), match.Any("processedAt"))
		})

	})

	Describe("GET /eventtypes/:eventTypeName/active", func() {
		It("should return the active event type", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventtypes/de-telekom-eni-quickstart-v1/active", nil)
			resp, err := ExecuteRequest(req, teamToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body, match.Any("status"))
		})
	})
})
