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

var _ = Describe("ApiExposure Controller", func() {

	Describe("GET /applications/:applicationId/apiexposures", func() {
		DescribeTable("should list API exposures with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apiexposures", nil)
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
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apiexposures", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Describe("GET /applications/:applicationId/apiexposures/:apiExposureName", func() {
		DescribeTable("should return a single API exposure with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apiexposures/eni-distr-v1", nil)
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
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apiexposures/eni-distr-v1", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

	})

	Describe("GET /applications/:applicationId/apiexposures/:apiExposureName/status", func() {
		DescribeTable("should return API exposure status with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apiexposures/eni-distr-v1/status", nil)
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
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apiexposures/eni-distr-v1/status", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

	})

	Describe("GET /applications/:applicationId/apiexposures/:apiExposureName/apisubscriptions", func() {
		DescribeTable("should list subscriptions for an API exposure with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apiexposures/eni-distr-v1/apisubscriptions", nil)
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
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/apiexposures/eni-distr-v1/apisubscriptions", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})
	})
})
