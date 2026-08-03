// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/rover-server/internal/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Resources Controller", func() {
	Context("GetAll resources", func() {
		DescribeTable("resolves exactly one team",
			func(path string, tokens []string, wantStatus int) {
				for _, token := range tokens {
					req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
					response, err := ExecuteRequest(req, token)
					contentType := "application/json"
					if wantStatus != http.StatusOK {
						contentType = "application/problem+json"
					}
					ExpectStatus(response, err, wantStatus, contentType)
				}
			},
			Entry("team derives scope", "/resources", []string{teamReadToken, teamToken}, http.StatusOK),
			Entry("team accepts matching scope", "/resources?group=eni&team=hyperion", []string{teamReadToken, teamToken}, http.StatusOK),
			Entry("team rejects partial scope", "/resources?group=eni", []string{teamReadToken, teamToken}, http.StatusBadRequest),
			Entry("team rejects another team", "/resources?group=eni&team=other", []string{teamReadToken, teamToken}, http.StatusForbidden),
			Entry("group requires scope", "/resources", []string{groupReadToken, groupToken}, http.StatusBadRequest),
			Entry("group accepts matching group", "/resources?group=eni&team=hyperion", []string{groupReadToken, groupToken}, http.StatusOK),
			Entry("group rejects another group", "/resources?group=other&team=hyperion", []string{groupReadToken, groupToken}, http.StatusForbidden),
			Entry("admin requires scope", "/resources", []string{adminReadToken, adminToken}, http.StatusBadRequest),
			Entry("admin selects a team", "/resources?group=eni&team=hyperion", []string{adminReadToken, adminToken}, http.StatusOK),
		)

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

	It("returns an internal server error without a security context", func() {
		_, err := NewResourcesController(stores).GetAll(context.Background(), api.GetAllResourcesParams{})

		var problem problems.Problem
		Expect(errors.As(err, &problem)).To(BeTrue())
		Expect(problem.Code()).To(Equal(http.StatusInternalServerError))
	})
})
