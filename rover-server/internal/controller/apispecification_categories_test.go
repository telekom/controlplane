// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/internal/api"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("fetchApiCategories", func() {
	newCategories := func(from, to int) []*apiv1.ApiCategory {
		items := make([]*apiv1.ApiCategory, 0, to-from)
		for i := from; i < to; i++ {
			name := fmt.Sprintf("cat-%03d", i)
			items = append(items, &apiv1.ApiCategory{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec:       apiv1.ApiCategorySpec{LabelValue: name, Active: true},
			})
		}
		return items
	}

	withLimit200 := mock.MatchedBy(func(opts store.ListOpts) bool {
		return opts.Limit == 200 && opts.Cursor == ""
	})

	It("fetches a single page of at most 200 ApiCategories", func() {
		categoryStore := mocks.NewMockObjectStore[*apiv1.ApiCategory](GinkgoT())
		categoryStore.EXPECT().List(mock.Anything, withLimit200).Return(&store.ListResponse[*apiv1.ApiCategory]{
			Items: newCategories(0, 200), Links: store.ListResponseLinks{Next: "cursor-1"},
		}, nil).Once()

		ctrl := &ApiSpecificationController{stores: &s.Stores{APICategoryStore: categoryStore}}
		list := ctrl.fetchApiCategories(context.Background())
		Expect(list.Items).To(HaveLen(200))

		_, ok := list.FindByLabelValue("cat-199")
		Expect(ok).To(BeTrue())
		_, ok = list.FindByLabelValue("cat-200")
		Expect(ok).To(BeFalse())
	})

	It("returns nil when the store is not configured", func() {
		ctrl := &ApiSpecificationController{stores: &s.Stores{}}
		Expect(ctrl.fetchApiCategories(context.Background())).To(BeNil())
	})

	It("returns nil when no ApiCategories exist", func() {
		categoryStore := mocks.NewMockObjectStore[*apiv1.ApiCategory](GinkgoT())
		categoryStore.EXPECT().List(mock.Anything, withLimit200).Return(&store.ListResponse[*apiv1.ApiCategory]{}, nil).Once()

		ctrl := &ApiSpecificationController{stores: &s.Stores{APICategoryStore: categoryStore}}
		Expect(ctrl.fetchApiCategories(context.Background())).To(BeNil())
	})

	It("returns nil when listing fails", func() {
		categoryStore := mocks.NewMockObjectStore[*apiv1.ApiCategory](GinkgoT())
		categoryStore.EXPECT().List(mock.Anything, withLimit200).Return(nil, fmt.Errorf("boom")).Once()

		ctrl := &ApiSpecificationController{stores: &s.Stores{APICategoryStore: categoryStore}}
		Expect(ctrl.fetchApiCategories(context.Background())).To(BeNil())
	})

	Context("Update", func() {
		var testCtx context.Context

		BeforeEach(func() {
			testCtx = security.ToContext(context.Background(), &security.BusinessContext{
				Environment: "poc", Group: "eni", Team: "hyperion",
			})
		})

		newUpdateRequest := func(category string) api.ApiSpecification {
			return api.ApiSpecification{Specification: map[string]any{
				"openapi": "3.0.0",
				"info": map[string]any{
					"title": "Test API", "version": "1.0.0", "x-api-category": category,
				},
				"servers": []any{map[string]any{"url": "http://example.com/eni/api/v1"}},
			}}
		}

		newCategoryStore := func() *mocks.MockObjectStore[*apiv1.ApiCategory] {
			categoryStore := mocks.NewMockObjectStore[*apiv1.ApiCategory](GinkgoT())
			categoryStore.EXPECT().List(mock.Anything, withLimit200).Return(&store.ListResponse[*apiv1.ApiCategory]{
				Items: newCategories(0, 200), Links: store.ListResponseLinks{Next: "cursor-1"},
			}, nil).Once()
			return categoryStore
		}

		It("accepts the 200th ApiCategory", func() {
			// The hash check runs only after category validation succeeded; stop the flow there.
			stopErr := fmt.Errorf("stop after validation")
			specStore := mocks.NewMockObjectStore[*roverv1.ApiSpecification](GinkgoT())
			specStore.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(nil, stopErr).Once()

			ctrl := &ApiSpecificationController{stores: &s.Stores{APICategoryStore: newCategoryStore()}, Store: specStore}
			_, err := ctrl.Update(testCtx, "eni--hyperion--test-api-v1", newUpdateRequest("cat-199"))
			Expect(err).To(MatchError(stopErr))
		})

		It("rejects an ApiCategory beyond the first 200", func() {
			ctrl := &ApiSpecificationController{stores: &s.Stores{APICategoryStore: newCategoryStore()}}
			_, err := ctrl.Update(testCtx, "eni--hyperion--test-api-v1", newUpdateRequest("cat-200"))
			problem, ok := err.(problems.Problem)
			Expect(ok).To(BeTrue())
			Expect(problem.Code()).To(Equal(http.StatusBadRequest))
		})
	})
})
