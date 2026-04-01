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

var _ = Describe("ApiSubscription Controller", func() {

	Describe("GET /applications/:applicationId/apisubscriptions", func() {
		DescribeTable("should list API subscriptions with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apisubscriptions", nil)
				resp, err := ExecuteRequest(req, token)
				body := ExpectStatusOk(resp, err)
				snaps.MatchJSON(GinkgoT(), body, match.Any("items.0.status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
			Entry("obfuscated scope", obfuscToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apisubscriptions", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Describe("GET /applications/:applicationId/apisubscriptions/:apiSubscriptionName", func() {
		DescribeTable("should return a single API subscription with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apisubscriptions/eni-distr-v1", nil)
				resp, err := ExecuteRequest(req, token)
				body := ExpectStatusOk(resp, err)
				snaps.MatchJSON(GinkgoT(), body, match.Any("status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
			Entry("obfuscated scope", obfuscToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apisubscriptions/eni-distr-v1", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

	})

	Describe("GET /applications/:applicationId/apisubscriptions/:apiSubscriptionName/status", func() {
		DescribeTable("should return API subscription status with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apisubscriptions/eni-distr-v1/status", nil)
				resp, err := ExecuteRequest(req, token)
				body := ExpectStatusOk(resp, err)
				snaps.MatchJSON(GinkgoT(), body, match.Any("createdAt"), match.Any("processedAt"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
			Entry("obfuscated scope", obfuscToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apisubscriptions/eni-distr-v1/status", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

	})
})
