// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/gkampitakis/go-snaps/match"
	"github.com/gkampitakis/go-snaps/snaps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	csstore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	"github.com/telekom/controlplane/discovery-server/internal/mapper"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// countingStore counts the items the wrapped store returns from List.
type countingStore[T csstore.Object] struct {
	csstore.ObjectStore[T]
	listed int
}

func (s *countingStore[T]) List(ctx context.Context, opts csstore.ListOpts) (*csstore.ListResponse[T], error) { //nolint:gocritic // hugeParam: signature is fixed by store.ObjectStore
	resp, err := s.ObjectStore.List(ctx, opts)
	if err == nil {
		s.listed += len(resp.Items)
	}
	return resp, err
}

func newExternalIdApplication(name, icto, psiid string) *applicationv1.Application {
	app := &applicationv1.Application{
		TypeMeta:   metav1.TypeMeta{APIVersion: applicationv1.GroupVersion.String(), Kind: "Application"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "poc--eni--hyperion"},
	}
	if icto != "" {
		app.Spec.ExternalIds = append(app.Spec.ExternalIds, applicationv1.ExternalId{Scheme: mapper.IctoScheme, Id: icto})
	}
	if psiid != "" {
		app.Spec.ExternalIds = append(app.Spec.ExternalIds, applicationv1.ExternalId{Scheme: mapper.PsiScheme, Id: psiid})
	}
	return app
}

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

		// The suite's application store is a mock that ignores ListOpts, so
		// filter semantics are covered against a real store below.
		DescribeTable("should keep external id filters in pagination links",
			func(query string) {
				req := httptest.NewRequest(http.MethodGet, "/applications?"+query, http.NoBody)
				resp, err := ExecuteRequest(req, adminToken)
				body := ExpectStatusOk(resp, err)

				var list api.ApplicationListResponse
				Expect(json.Unmarshal([]byte(body), &list)).To(Succeed())
				Expect(list.UnderscoreLinks.Self).To(HavePrefix("/applications?" + query + "&"))
			},
			Entry("icto", "icto=icto-12345"),
			Entry("psiid", "psiid=PSI-123456"),
			Entry("icto and psiid", "icto=icto-12345&psiid=PSI-123456"),
		)

		Context("against a real store", Ordered, func() {
			var (
				ctrl *applicationController
				apps *countingStore[*applicationv1.Application]
			)

			BeforeAll(func() {
				storeCtx, cancel := context.WithCancel(context.Background())
				DeferCleanup(cancel)
				s := newCacheStore[*applicationv1.Application](storeCtx, applicationv1.GroupVersion.WithKind("Application"), "applications")
				seed(storeCtx, s, newExternalIdApplication("both", "icto-1", "psi-1"))
				seed(storeCtx, s, newExternalIdApplication("icto-only", "icto-1", ""))
				seed(storeCtx, s, newExternalIdApplication("other-ids", "icto-2", "psi-2"))
				seed(storeCtx, s, newExternalIdApplication("no-ids", "", ""))
				apps = &countingStore[*applicationv1.Application]{ObjectStore: s}
				ctrl = &applicationController{stores: &sstore.Stores{ApplicationStore: apps}}
			})

			DescribeTable("should filter by external ids inside the store",
				func(params api.GetAllApplicationsParams, expected ...string) {
					apps.listed = 0
					resp, err := ctrl.GetAll(context.Background(), params)
					Expect(err).NotTo(HaveOccurred())

					names := make([]string, 0, len(resp.Items))
					for _, item := range resp.Items {
						names = append(names, item.Name)
					}
					Expect(names).To(ConsistOf(expected))
					Expect(resp.Paging.Total).To(Equal(len(expected)))
					// Non-matching applications must never leave the store.
					Expect(apps.listed).To(Equal(len(expected)))
				},
				Entry("no filters", api.GetAllApplicationsParams{}, "both", "icto-only", "other-ids", "no-ids"),
				Entry("icto", api.GetAllApplicationsParams{Icto: "icto-1"}, "both", "icto-only"),
				Entry("psiid", api.GetAllApplicationsParams{Psiid: "psi-1"}, "both"),
				Entry("icto and psiid", api.GetAllApplicationsParams{Icto: "icto-1", Psiid: "psi-1"}, "both"),
				Entry("icto with non-matching psiid", api.GetAllApplicationsParams{Icto: "icto-1", Psiid: "psi-2"}),
				Entry("non-matching icto", api.GetAllApplicationsParams{Icto: "icto-9"}),
				Entry("psiid holding another scheme's id", api.GetAllApplicationsParams{Psiid: "icto-1"}),
			)
		})
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
