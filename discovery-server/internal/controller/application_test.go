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

var _ = Describe("Application Controller", func() {

	Describe("GET /applications", func() {
		DescribeTable("should list applications with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications", nil)
				resp, err := ExecuteRequest(req, token)
				body := ExpectStatusOk(resp, err)
				snaps.MatchJSON(GinkgoT(), body, match.Any("items.0.status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return applications for a different team (empty if no data)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body)
		})

		It("should return applications for a different group (empty if no data)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications", nil)
			resp, err := ExecuteRequest(req, groupOtherToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body)
		})

		It("should return applications for a partial team prefix (empty if no data)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications", nil)
			resp, err := ExecuteRequest(req, teamPrefixToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body)
		})

		It("should return applications for a partial group prefix (empty if no data)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications", nil)
			resp, err := ExecuteRequest(req, groupPrefixToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body)
		})
	})

	Describe("GET /applications/:applicationId", func() {
		DescribeTable("should return an application with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", nil)
				resp, err := ExecuteRequest(req, token)
				body := ExpectStatusOk(resp, err)
				snaps.MatchJSON(GinkgoT(), body, match.Any("status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a different group", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", nil)
			resp, err := ExecuteRequest(req, groupOtherToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a partial team name prefix (hyper != hyperion)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", nil)
			resp, err := ExecuteRequest(req, teamPrefixToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a partial group name prefix (en != eni)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", nil)
			resp, err := ExecuteRequest(req, groupPrefixToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

	})

	Describe("GET /applications/:applicationId/status", func() {
		DescribeTable("should return application status with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/status", nil)
				resp, err := ExecuteRequest(req, token)
				body := ExpectStatusOk(resp, err)
				snaps.MatchJSON(GinkgoT(), body, match.Any("createdAt"), match.Any("processedAt"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/status", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a different group", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/status", nil)
			resp, err := ExecuteRequest(req, groupOtherToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a partial team name prefix (hyper != hyperion)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/status", nil)
			resp, err := ExecuteRequest(req, teamPrefixToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a partial group name prefix (en != eni)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/status", nil)
			resp, err := ExecuteRequest(req, groupPrefixToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

	})
})
