// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Resources Controller", func() {
	Context("GetAll resources", func() {
		It("should return aggregated resources for a group token", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources", nil)

			response, err := ExecuteRequest(req, groupToken)
			ExpectStatusOk(response, err)
		})

		It("should return aggregated resources for a team token", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources", nil)

			response, err := ExecuteRequest(req, teamToken)
			ExpectStatusOk(response, err)
		})

		It("should return an empty list for a team with no resources", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources", nil)

			response, err := ExecuteRequest(req, teamNoResources)
			ExpectStatusWithBody(response, err, http.StatusOK, "application/json")
		})

		It("should filter results when prefix query param is provided", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?prefix=poc--eni--hyperion--", nil)

			response, err := ExecuteRequest(req, groupToken)
			ExpectStatusOk(response, err)
		})

		It("should return 403 when prefix is outside caller scope", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?prefix=poc--other--team--", nil)

			response, err := ExecuteRequest(req, teamToken)
			ExpectStatus(response, err, http.StatusForbidden, "application/problem+json")
		})
	})
})
