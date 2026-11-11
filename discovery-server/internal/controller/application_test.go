// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/gkampitakis/go-snaps/match"
	"github.com/gkampitakis/go-snaps/snaps"

	"github.com/telekom/controlplane/discovery-server/internal/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Application Controller", func() {
	Describe("GET /applications", func() {
		DescribeTable("should list applications with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications", http.NoBody)
				resp, err := ExecuteRequest(req, token)
				body := ExpectStatusOk(resp, err)
				snaps.MatchJSON(GinkgoT(), body, match.Any("items.0.status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return applications for a different team (empty if no data)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications", http.NoBody)
			resp, err := ExecuteRequest(req, teamNoResToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body)
		})

		It("should return applications for a different group (empty if no data)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications", http.NoBody)
			resp, err := ExecuteRequest(req, groupOtherToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body)
		})

		It("should return applications for a partial team prefix (empty if no data)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications", http.NoBody)
			resp, err := ExecuteRequest(req, teamPrefixToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body)
		})

		It("should return applications for a partial group prefix (empty if no data)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications", http.NoBody)
			resp, err := ExecuteRequest(req, groupPrefixToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body)
		})

		DescribeTable("should filter applications by external ids",
			func(query string, expectedCount int) {
				req := httptest.NewRequest(http.MethodGet, "/applications?"+query, http.NoBody)
				resp, err := ExecuteRequest(req, adminToken)
				body := ExpectStatusOk(resp, err)

				Expect(resp.Header.Get("X-Total-Count")).To(Equal(strconv.Itoa(expectedCount)))

				var list api.ApplicationListResponse
				Expect(json.Unmarshal([]byte(body), &list)).To(Succeed())
				Expect(list.Items).To(HaveLen(expectedCount))
				Expect(list.UnderscoreLinks.Self).To(HavePrefix("/applications?" + query + "&"))
				for _, item := range list.Items {
					Expect(item.Icto).To(Equal("icto-12345"))
					Expect(item.Psiid).To(Equal("PSI-123456"))
				}
			},
			Entry("matching icto", "icto=icto-12345", 1),
			Entry("non-matching icto", "icto=icto-99999", 0),
			Entry("matching psiid", "psiid=PSI-123456", 1),
			Entry("non-matching psiid", "psiid=PSI-999999", 0),
			Entry("matching icto and psiid", "icto=icto-12345&psiid=PSI-123456", 1),
			Entry("matching icto, non-matching psiid", "icto=icto-12345&psiid=PSI-999999", 0),
		)
	})

	Describe("GET /applications/:applicationId", func() {
		DescribeTable("should return an application with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", http.NoBody)
				resp, err := ExecuteRequest(req, token)
				body := ExpectStatusOk(resp, err)
				snaps.MatchJSON(GinkgoT(), body, match.Any("status"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", http.NoBody)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a different group", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", http.NoBody)
			resp, err := ExecuteRequest(req, groupOtherToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a partial team name prefix (hyper != hyperion)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", http.NoBody)
			resp, err := ExecuteRequest(req, teamPrefixToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a partial group name prefix (en != eni)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", http.NoBody)
			resp, err := ExecuteRequest(req, groupPrefixToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})
	})

	Describe("GET /applications/:applicationId/status", func() {
		DescribeTable("should return application status with different scopes",
			func(token string) {
				req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/status", http.NoBody)
				resp, err := ExecuteRequest(req, token)
				body := ExpectStatusOk(resp, err)
				snaps.MatchJSON(GinkgoT(), body, match.Any("createdAt"), match.Any("processedAt"))
			},
			Entry("team scope", teamToken),
			Entry("group scope", groupToken),
			Entry("admin scope", adminToken),
		)

		It("should return 403 for a different team", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/status", http.NoBody)
			resp, err := ExecuteRequest(req, teamNoResToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a different group", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/status", http.NoBody)
			resp, err := ExecuteRequest(req, groupOtherToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a partial team name prefix (hyper != hyperion)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/status", http.NoBody)
			resp, err := ExecuteRequest(req, teamPrefixToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})

		It("should return 403 for a partial group name prefix (en != eni)", func() {
			req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app/status", http.NoBody)
			resp, err := ExecuteRequest(req, groupPrefixToken)
			ExpectStatus(resp, err, http.StatusForbidden, "application/problem+json")
		})
	})
})
