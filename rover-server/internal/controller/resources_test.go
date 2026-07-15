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
			req := httptest.NewRequest(http.MethodGet, "/resources?group=eni&team=hyperion", nil)

			response, err := ExecuteRequest(req, groupToken)
			ExpectStatusOk(response, err)
		})

		It("should return aggregated resources for a team token", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?group=eni&team=hyperion", nil)

			response, err := ExecuteRequest(req, teamToken)
			ExpectStatusOk(response, err)
		})

		It("should return an empty list for a team with no resources", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?group=eni&team=nohyper", nil)

			response, err := ExecuteRequest(req, teamNoResources)
			ExpectStatusWithBody(response, err, http.StatusOK, "application/json")
		})

		It("should return 400 when group is missing", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?team=hyperion", nil)

			response, err := ExecuteRequest(req, groupToken)
			ExpectStatus(response, err, http.StatusBadRequest, "application/problem+json")
		})

		It("should return 400 when team is missing", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?group=eni", nil)

			response, err := ExecuteRequest(req, groupToken)
			ExpectStatus(response, err, http.StatusBadRequest, "application/problem+json")
		})

		It("should return 403 when group/team is outside caller scope", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?group=other&team=team", nil)

			response, err := ExecuteRequest(req, teamToken)
			ExpectStatus(response, err, http.StatusForbidden, "application/problem+json")
		})
	})
})
