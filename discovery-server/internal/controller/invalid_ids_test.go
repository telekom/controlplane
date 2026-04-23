// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Controller Invalid IDs", func() {
	// invalidApplicationID has the correct group--team--app format so it passes
	// the security middleware, but "invalid-id" doesn't match any fixture label
	// ("my-app"), causing VerifyApplicationLabel to return 404 for single-resource
	// endpoints. List endpoints and application get/status do not call
	// VerifyApplicationLabel and therefore return 200 — those are tested separately.
	const invalidApplicationID = "eni--hyperion--invalid-id"

	// invalidEventTypeID lacks the required group--team--name format, so
	// ParseResourceId returns 400 before the security middleware resolves a namespace.
	const invalidEventTypeID = "invalid-id"

	DescribeTable("should return 404 when app name does not match resource label",
		func(path string) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp, err := ExecuteRequest(req, teamToken)
			ExpectStatus(resp, err, http.StatusNotFound, "application/problem+json")
		},
		Entry("api exposures get", "/applications/"+invalidApplicationID+"/apiexposures/eni-distr-v1"),
		Entry("api exposures status", "/applications/"+invalidApplicationID+"/apiexposures/eni-distr-v1/status"),
		Entry("api exposure subscriptions", "/applications/"+invalidApplicationID+"/apiexposures/eni-distr-v1/apisubscriptions"),
		Entry("api subscriptions get", "/applications/"+invalidApplicationID+"/apisubscriptions/eni-distr-v1"),
		Entry("api subscriptions status", "/applications/"+invalidApplicationID+"/apisubscriptions/eni-distr-v1/status"),
		Entry("event exposures get", "/applications/"+invalidApplicationID+"/eventexposures/de-telekom-eni-quickstart-v1"),
		Entry("event exposures status", "/applications/"+invalidApplicationID+"/eventexposures/de-telekom-eni-quickstart-v1/status"),
		Entry("event exposure subscriptions", "/applications/"+invalidApplicationID+"/eventexposures/de-telekom-eni-quickstart-v1/eventsubscriptions"),
		Entry("event subscriptions get", "/applications/"+invalidApplicationID+"/eventsubscriptions/de-telekom-eni-quickstart-v1"),
		Entry("event subscriptions status", "/applications/"+invalidApplicationID+"/eventsubscriptions/de-telekom-eni-quickstart-v1/status"),
	)

	DescribeTable("should return 400 for malformed event type ID",
		func(path string) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp, err := ExecuteRequest(req, teamToken)
			ExpectStatus(resp, err, http.StatusBadRequest, "application/problem+json")
		},
		Entry("event type get", "/eventtypes/"+invalidEventTypeID),
		Entry("event type status", "/eventtypes/"+invalidEventTypeID+"/status"),
	)
})
