// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/stretchr/testify/mock"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

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
			Entry("team derives scope", "/resources?limit=1", []string{teamReadToken, teamToken}, http.StatusOK),
			Entry("team accepts matching scope", "/resources?group=eni&team=hyperion&limit=1", []string{teamReadToken, teamToken}, http.StatusOK),
			Entry("team rejects partial scope", "/resources?group=eni", []string{teamReadToken, teamToken}, http.StatusBadRequest),
			Entry("team rejects another team", "/resources?group=eni&team=other", []string{teamReadToken, teamToken}, http.StatusForbidden),
			Entry("group requires scope", "/resources", []string{groupReadToken, groupToken}, http.StatusBadRequest),
			Entry("group accepts matching group", "/resources?group=eni&team=hyperion&limit=1", []string{groupReadToken, groupToken}, http.StatusOK),
			Entry("group rejects another group", "/resources?group=other&team=hyperion", []string{groupReadToken, groupToken}, http.StatusForbidden),
			Entry("admin requires scope", "/resources", []string{adminReadToken, adminToken}, http.StatusBadRequest),
			Entry("admin selects a team", "/resources?group=eni&team=hyperion&limit=1", []string{adminReadToken, adminToken}, http.StatusOK),
		)

		It("should return aggregated resources for a group token", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?group=eni&team=hyperion", http.NoBody)

			response, err := ExecuteRequest(req, groupToken)
			ExpectStatus(response, err, http.StatusOK, "application/json")
			self := parseResourceLink(decodeResourceResponse(response).UnderscoreLinks.Self)
			Expect(self.Query().Get("limit")).To(Equal("20"))
		})

		It("should return aggregated resources for a team token", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?group=eni&team=hyperion", http.NoBody)

			response, err := ExecuteRequest(req, teamToken)
			ExpectStatus(response, err, http.StatusOK, "application/json")
		})

		It("builds links for an explicit team selection", func() {
			response, err := ExecuteRequest(httptest.NewRequest(http.MethodGet, "/resources?group=eni&team=hyperion&limit=1", http.NoBody), teamToken)
			ExpectStatus(response, err, http.StatusOK, "application/json")
			page := decodeResourceResponse(response)

			self := parseResourceLink(page.UnderscoreLinks.Self)
			Expect(self.Query()).To(Equal(url.Values{"group": {"eni"}, "team": {"hyperion"}, "limit": {"1"}}))

			next := parseResourceLink(page.UnderscoreLinks.Next)
			Expect(next.Query()["cursor"]).To(HaveLen(1))
			Expect(next.Query().Get("cursor")).NotTo(BeEmpty())
			Expect(next.Query().Get("group")).To(Equal("eni"))
			Expect(next.Query().Get("team")).To(Equal("hyperion"))
			Expect(next.Query().Get("limit")).To(Equal("1"))

			response, err = ExecuteRequest(httptest.NewRequest(http.MethodGet, next.RequestURI(), http.NoBody), teamToken)
			ExpectStatus(response, err, http.StatusOK, "application/json")
			followingPage := decodeResourceResponse(response)
			followingSelf := parseResourceLink(followingPage.UnderscoreLinks.Self)
			Expect(followingSelf.Query()["cursor"]).To(ConsistOf(next.Query().Get("cursor")))
		})

		It("builds resolved links when a team token omits selection", func() {
			response, err := ExecuteRequest(httptest.NewRequest(http.MethodGet, "/resources?limit=1", http.NoBody), teamToken)
			ExpectStatus(response, err, http.StatusOK, "application/json")
			page := decodeResourceResponse(response)

			self := parseResourceLink(page.UnderscoreLinks.Self)
			Expect(self.Query()).To(Equal(url.Values{"group": {"eni"}, "team": {"hyperion"}, "limit": {"1"}}))
			next := parseResourceLink(page.UnderscoreLinks.Next)
			Expect(next.Query().Get("group")).To(Equal("eni"))
			Expect(next.Query().Get("team")).To(Equal("hyperion"))
			Expect(next.Query().Get("limit")).To(Equal("1"))
		})

		It("should return an empty list for a team with no resources", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?group=eni&team=nohyper", http.NoBody)

			response, err := ExecuteRequest(req, teamNoResources)
			ExpectStatus(response, err, http.StatusOK, "application/json")
		})

		It("should return 400 when group is missing", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?team=hyperion", http.NoBody)

			response, err := ExecuteRequest(req, groupToken)
			ExpectStatus(response, err, http.StatusBadRequest, "application/problem+json")
		})

		It("should return 400 when team is missing", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?group=eni", http.NoBody)

			response, err := ExecuteRequest(req, groupToken)
			ExpectStatus(response, err, http.StatusBadRequest, "application/problem+json")
		})

		It("should return 403 when group/team is outside caller scope", func() {
			req := httptest.NewRequest(http.MethodGet, "/resources?group=other&team=team", http.NoBody)

			response, err := ExecuteRequest(req, teamToken)
			ExpectStatus(response, err, http.StatusForbidden, "application/problem+json")
		})

		It("does not expose internal store errors", func() {
			original := stores.RoverStore
			DeferCleanup(func() { stores.RoverStore = original })
			failedStore := mocks.NewMockObjectStore[*roverv1.Rover](GinkgoT())
			failedStore.EXPECT().List(mock.Anything, mock.Anything).Return((*store.ListResponse[*roverv1.Rover])(nil), errors.New("secret datastore detail"))
			stores.RoverStore = failedStore

			response, err := ExecuteRequest(httptest.NewRequest(http.MethodGet, "/resources?group=eni&team=hyperion", http.NoBody), teamToken)
			ExpectStatus(response, err, http.StatusInternalServerError, "application/problem+json")
			body, err := io.ReadAll(response.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(body).NotTo(ContainSubstring("secret datastore detail"))
		})
	})

	It("returns an internal server error without a security context", func() {
		_, err := NewResourcesController(stores).GetAll(context.Background(), api.GetAllResourcesParams{})

		var problem problems.Problem
		Expect(errors.As(err, &problem)).To(BeTrue())
		Expect(problem.Code()).To(Equal(http.StatusInternalServerError))
	})
})

func decodeResourceResponse(response *http.Response) api.ResourceListResponse {
	var page api.ResourceListResponse
	Expect(json.NewDecoder(response.Body).Decode(&page)).To(Succeed())
	return page
}

func parseResourceLink(link string) *url.URL {
	parsed, err := url.Parse(link)
	Expect(err).NotTo(HaveOccurred())
	Expect(parsed.IsAbs()).To(BeTrue())
	Expect(parsed.Path).To(Equal("/resources"))
	return parsed
}
