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

var _ = Describe("EventSubscription Controller", func() {

	Describe("GET /applications/:applicationId/eventsubscriptions", func() {
		DescribeTable("should list event subscriptions with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventsubscriptions", nil)
				resp, err := ExecuteRequest(req, token)
				ExpectStatusOk(resp, err, match.Any("items.0.status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventsubscriptions", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Describe("GET /applications/:applicationId/eventsubscriptions/:eventSubscriptionName", func() {
		DescribeTable("should return a single event subscription with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventsubscriptions/de-telekom-eni-quickstart-v1", nil)
				resp, err := ExecuteRequest(req, token)
				ExpectStatusOk(resp, err, match.Any("status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventsubscriptions/de-telekom-eni-quickstart-v1", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

	})

	Describe("GET /applications/:applicationId/eventsubscriptions/:eventSubscriptionName/status", func() {
		DescribeTable("should return event subscription status with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventsubscriptions/de-telekom-eni-quickstart-v1/status", nil)
				resp, err := ExecuteRequest(req, token)
				ExpectStatusOk(resp, err, match.Any("createdAt"), match.Any("processedAt"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventsubscriptions/de-telekom-eni-quickstart-v1/status", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

	})
})
