// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Deprecated Endpoints", func() {

	DescribeTable("should return 410 Gone for deprecated write endpoints",
		func(method, path string) {
			req := httptest.NewRequest(method, path, nil)
			resp, err := ExecuteRequest(req, teamToken)
			ExpectStatus(resp, err, http.StatusGone, "application/problem+json")
		},

		// Application write endpoints
		Entry("POST /applications",
			http.MethodPost, "/applications"),
		Entry("PUT /applications/:applicationId",
			http.MethodPut, "/applications/eni--hyperion--my-app"),
		Entry("DELETE /applications/:applicationId",
			http.MethodDelete, "/applications/eni--hyperion--my-app"),

		// ApiExposure write endpoints
		Entry("POST /applications/:applicationId/apiexposures",
			http.MethodPost, "/applications/eni--hyperion--my-app/apiexposures"),
		Entry("PUT /applications/:applicationId/apiexposures/:apiExposureName",
			http.MethodPut, "/applications/eni--hyperion--my-app/apiexposures/eni-distr-v1"),
		Entry("DELETE /applications/:applicationId/apiexposures/:apiExposureName",
			http.MethodDelete, "/applications/eni--hyperion--my-app/apiexposures/eni-distr-v1"),

		// ApiSubscription write endpoints
		Entry("POST /applications/:applicationId/apisubscriptions",
			http.MethodPost, "/applications/eni--hyperion--my-app/apisubscriptions"),
		Entry("PUT /applications/:applicationId/apisubscriptions/:apiSubscriptionName",
			http.MethodPut, "/applications/eni--hyperion--my-app/apisubscriptions/eni-distr-v1"),
		Entry("DELETE /applications/:applicationId/apisubscriptions/:apiSubscriptionName",
			http.MethodDelete, "/applications/eni--hyperion--my-app/apisubscriptions/eni-distr-v1"),

		// ApiSubscription approve endpoint
		Entry("POST /applications/:applicationId/apisubscriptions/:apiSubscriptionName/approve",
			http.MethodPost, "/applications/eni--hyperion--my-app/apisubscriptions/eni-distr-v1/approve"),

		// EventType write endpoint
		Entry("POST /eventtypes",
			http.MethodPost, "/eventtypes"),

		// EventExposure write endpoints
		Entry("POST /applications/:applicationId/eventexposures",
			http.MethodPost, "/applications/eni--hyperion--my-app/eventexposures"),
		Entry("PUT /applications/:applicationId/eventexposures/:eventExposureName",
			http.MethodPut, "/applications/eni--hyperion--my-app/eventexposures/de-telekom-eni-quickstart-v1"),
		Entry("DELETE /applications/:applicationId/eventexposures/:eventExposureName",
			http.MethodDelete, "/applications/eni--hyperion--my-app/eventexposures/de-telekom-eni-quickstart-v1"),

		// EventSubscription write endpoints
		Entry("POST /applications/:applicationId/eventsubscriptions",
			http.MethodPost, "/applications/eni--hyperion--my-app/eventsubscriptions"),
		Entry("PUT /applications/:applicationId/eventsubscriptions/:eventSubscriptionName",
			http.MethodPut, "/applications/eni--hyperion--my-app/eventsubscriptions/de-telekom-eni-quickstart-v1"),
		Entry("DELETE /applications/:applicationId/eventsubscriptions/:eventSubscriptionName",
			http.MethodDelete, "/applications/eni--hyperion--my-app/eventsubscriptions/de-telekom-eni-quickstart-v1"),
	)
})
