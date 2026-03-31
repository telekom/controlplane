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

var _ = Describe("EventExposure Controller", func() {

	Describe("GET /applications/:applicationId/eventexposures", func() {
		DescribeTable("should list event exposures with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventexposures", nil)
				resp, err := ExecuteRequest(req, token)
				ExpectStatusOk(resp, err, match.Any("items.0.status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventexposures", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Describe("GET /applications/:applicationId/eventexposures/:eventExposureName", func() {
		DescribeTable("should return a single event exposure with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventexposures/de-telekom-eni-quickstart-v1", nil)
				resp, err := ExecuteRequest(req, token)
				ExpectStatusOk(resp, err, match.Any("status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventexposures/de-telekom-eni-quickstart-v1", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

	})

	Describe("GET /applications/:applicationId/eventexposures/:eventExposureName/status", func() {
		DescribeTable("should return event exposure status with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventexposures/de-telekom-eni-quickstart-v1/status", nil)
				resp, err := ExecuteRequest(req, token)
				ExpectStatusOk(resp, err, match.Any("createdAt"), match.Any("processedAt"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventexposures/de-telekom-eni-quickstart-v1/status", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

	})

	Describe("GET /applications/:applicationId/eventexposures/:eventExposureName/eventsubscriptions", func() {
		DescribeTable("should list subscriptions for an event exposure with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventexposures/de-telekom-eni-quickstart-v1/eventsubscriptions", nil)
				resp, err := ExecuteRequest(req, token)
				ExpectStatusOk(resp, err, match.Any("items.0.status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/eventexposures/de-telekom-eni-quickstart-v1/eventsubscriptions", nil)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

	})
})
