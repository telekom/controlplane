// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/gkampitakis/go-snaps/match"
	"github.com/gkampitakis/go-snaps/snaps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
	eventv1 "github.com/telekom/controlplane/event/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EventType Controller", func() {
	Describe("GET /eventtypes", func() {
		It("should list event types with team token", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventtypes", http.NoBody)
			resp, err := ExecuteRequest(req, teamToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body, match.Any("items.0.status"))
		})

		It("should list event types with admin token", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventtypes", http.NoBody)
			resp, err := ExecuteRequest(req, adminToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body, match.Any("items.0.status"))
		})

		It("lists across teams but excludes other environments", func() {
			storeCtx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			s := newCacheStore[*eventv1.EventType](storeCtx, eventv1.GroupVersion.WithKind("EventType"), "eventtypes")
			for name, ns := range map[string]string{
				"same-team":  "poc--eni--hyperion",
				"other-team": "poc--other--team",
				"other-env":  "prod--eni--hyperion",
				"prefix-env": "poc-other--eni--hyperion",
			} {
				seed(storeCtx, s, &eventv1.EventType{
					TypeMeta:   metav1.TypeMeta{APIVersion: eventv1.GroupVersion.String(), Kind: "EventType"},
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				})
			}
			ctrl := NewEventTypeController(&sstore.Stores{EventTypeStore: s})
			reqCtx := security.ToContext(context.Background(), &security.BusinessContext{Environment: "poc"})

			resp, err := ctrl.GetAll(reqCtx, api.GetAllEventTypesParams{})

			Expect(err).NotTo(HaveOccurred())
			names := make([]string, 0, len(resp.Items))
			for _, item := range resp.Items {
				names = append(names, item.Name)
			}
			Expect(names).To(ConsistOf("same-team", "other-team"))
		})

		DescribeTable("rejects a missing environment before reading the store",
			func(reqCtx context.Context) {
				ctrl := NewEventTypeController(&sstore.Stores{})
				resp, err := ctrl.GetAll(reqCtx, api.GetAllEventTypesParams{})
				var problem problems.Problem
				Expect(errors.As(err, &problem)).To(BeTrue())
				Expect(problem.Code()).To(Equal(http.StatusInternalServerError))
				Expect(resp).To(BeNil())
			},
			Entry("no security context", context.Background()),
			Entry("nil security context", security.ToContext(context.Background(), nil)),
			Entry("empty environment", security.ToContext(context.Background(), &security.BusinessContext{})),
		)
	})

	Describe("GET /eventtypes/:eventTypeId", func() {
		It("should return a single event type", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventtypes/eni--hyperion--de-telekom-eni-quickstart-v1", http.NoBody)
			resp, err := ExecuteRequest(req, teamToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body, match.Any("status"))
		})
	})

	Describe("GET /eventtypes/:eventTypeId/status", func() {
		It("should return event type status", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventtypes/eni--hyperion--de-telekom-eni-quickstart-v1/status", http.NoBody)
			resp, err := ExecuteRequest(req, teamToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body, match.Any("createdAt"), match.Any("processedAt"))
		})
	})

	Describe("GET /eventtypes/:eventTypeName/active", func() {
		It("should return the active event type", func() {
			req := httptest.NewRequest(http.MethodGet, "/eventtypes/de-telekom-eni-quickstart-v1/active", http.NoBody)
			resp, err := ExecuteRequest(req, teamToken)
			body := ExpectStatusOk(resp, err)
			snaps.MatchJSON(GinkgoT(), body, match.Any("status"))
		})
	})
})
