// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"net/http"
	"net/http/httptest"

	"github.com/gkampitakis/go-snaps/match"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Application Controller", func() {

	Describe("GET /applications", func() {
		DescribeTable("should list applications with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications", nil)
				resp, err := ExecuteRequest(req, token)
				ExpectStatusOk(resp, err, match.Any("items.0.status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return applications for a different team (empty if no data)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatusOk(resp, err)
		})
	})

	Describe("GET /applications/:applicationId", func() {
		DescribeTable("should return an application with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", nil)
				resp, err := ExecuteRequest(req, token)
				ExpectStatusOk(resp, err, match.Any("status"))
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

	})

	Describe("GET /applications/:applicationId/status", func() {
		DescribeTable("should return application status with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/status", nil)
				resp, err := ExecuteRequest(req, token)
				ExpectStatusOk(resp, err, match.Any("createdAt"), match.Any("processedAt"))
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

	})
})
