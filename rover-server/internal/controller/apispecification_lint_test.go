// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	fileApi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/oaslint"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockLinter is a simple test double for oaslint.Linter that records whether Lint was called.
type mockLinter struct {
	called  bool
	outcome oaslint.Outcome
	err     error
}

func (m *mockLinter) Lint(_ context.Context, apiSpec *roverv1.ApiSpecification, _ *apiv1.ApiCategory, _ io.Reader) (oaslint.Outcome, error) {
	m.called = true
	if apiSpec.Spec.Lint == nil {
		apiSpec.Spec.Lint = &roverv1.LintResult{Passed: true, Message: "mock lint ran"}
	}
	return m.outcome, m.err
}

var _ = Describe("Linter unreachable error propagation to user", func() {
	var (
		ctrl    *ApiSpecificationController
		testCtx context.Context
	)

	newCategoryWithMode := func(mode apiv1.LintingMode) *apiv1.ApiCategory {
		return &apiv1.ApiCategory{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cat"},
			Spec: apiv1.ApiCategorySpec{
				LabelValue: "test-cat",
				Active:     true,
				Linting: &apiv1.LintingConfig{
					Mode:    mode,
					Ruleset: "default",
				},
			},
		}
	}

	newUpdateRequest := func() api.ApiSpecification {
		return api.ApiSpecification{
			Specification: map[string]interface{}{
				"openapi": "3.0.0",
				"info": map[string]interface{}{
					"title":          "Test API",
					"version":        "1.0.0",
					"x-api-category": "test-cat",
				},
				"servers": []interface{}{
					map[string]interface{}{"url": "http://example.com/test/api/v1"},
				},
			},
		}
	}

	BeforeEach(func() {
		bCtx := &security.BusinessContext{
			Environment: "poc",
			Group:       "eni",
			Team:        "hyperion",
		}
		testCtx = security.ToContext(context.Background(), bCtx)
	})

	Context("when linter returns an error and category mode is Block", func() {
		It("should return InternalServerError to the user", func() {
			categoryStore := mocks.NewMockObjectStore[*apiv1.ApiCategory](GinkgoT())
			categoryStore.EXPECT().List(mock.Anything, mock.Anything).Return(
				&store.ListResponse[*apiv1.ApiCategory]{Items: []*apiv1.ApiCategory{
					newCategoryWithMode(apiv1.LintingModeBlock),
				}}, nil)

			specStore := mocks.NewMockObjectStore[*roverv1.ApiSpecification](GinkgoT())
			specStore.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(
				nil, problems.NotFound())

			ctrl = &ApiSpecificationController{
				stores: &s.Stores{APICategoryStore: categoryStore},
				Store:  specStore,
				Linter: &mockLinter{outcome: oaslint.Completed, err: fmt.Errorf("connection refused")},
			}

			_, err := ctrl.Update(testCtx, "eni--hyperion--test-api-v1", newUpdateRequest())
			Expect(err).To(HaveOccurred())

			problem, ok := err.(problems.Problem)
			Expect(ok).To(BeTrue(), "error should be a problems.Problem")
			Expect(problem.Code()).To(Equal(http.StatusInternalServerError))
			Expect(problem.Error()).To(ContainSubstring("linting service could not be reached"))
		})
	})

	Context("when linter returns an error and category mode is Warn", func() {
		It("should NOT reject the request at the linting stage", func() {
			categoryStore := mocks.NewMockObjectStore[*apiv1.ApiCategory](GinkgoT())
			categoryStore.EXPECT().List(mock.Anything, mock.Anything).Return(
				&store.ListResponse[*apiv1.ApiCategory]{Items: []*apiv1.ApiCategory{
					newCategoryWithMode(apiv1.LintingModeWarn),
				}}, nil)

			specStore := mocks.NewMockObjectStore[*roverv1.ApiSpecification](GinkgoT())
			specStore.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(
				nil, problems.NotFound())
			specStore.EXPECT().CreateOrReplace(mock.Anything, mock.Anything).Return(nil)

			mockFileManager.EXPECT().UploadFile(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
				&fileApi.FileUploadResponse{FileHash: "abc", FileId: "test-id", ContentType: "application/yaml"}, nil)

			ctrl = &ApiSpecificationController{
				stores: &s.Stores{APICategoryStore: categoryStore},
				Store:  specStore,
				Linter: &mockLinter{outcome: oaslint.Completed, err: fmt.Errorf("connection refused")},
			}

			_, err := ctrl.Update(testCtx, "eni--hyperion--test-api-v1", newUpdateRequest())
			// The request passes the linting check. It may fail later for unrelated reasons
			// (e.g., the Get call in the response mapping). The critical assertion:
			// the error is NOT a 500 "Linting failed" problem.
			if err != nil {
				problem, ok := err.(problems.Problem)
				if ok {
					Expect(problem.Code()).ToNot(Equal(http.StatusInternalServerError))
					Expect(problem.Error()).ToNot(ContainSubstring("Linting failed"))
				}
			}
		})
	})

	Context("when linter returns Blocked", func() {
		It("should return BadRequest to the user", func() {
			categoryStore := mocks.NewMockObjectStore[*apiv1.ApiCategory](GinkgoT())
			categoryStore.EXPECT().List(mock.Anything, mock.Anything).Return(
				&store.ListResponse[*apiv1.ApiCategory]{Items: []*apiv1.ApiCategory{
					newCategoryWithMode(apiv1.LintingModeBlock),
				}}, nil)

			specStore := mocks.NewMockObjectStore[*roverv1.ApiSpecification](GinkgoT())
			specStore.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(
				nil, problems.NotFound())

			ctrl = &ApiSpecificationController{
				stores: &s.Stores{APICategoryStore: categoryStore},
				Store:  specStore,
				Linter: &mockLinter{outcome: oaslint.Blocked, err: nil},
			}

			_, err := ctrl.Update(testCtx, "eni--hyperion--test-api-v1", newUpdateRequest())
			Expect(err).To(HaveOccurred())

			problem, ok := err.(problems.Problem)
			Expect(ok).To(BeTrue(), "error should be a problems.Problem")
			Expect(problem.Code()).To(Equal(http.StatusBadRequest))
			Expect(problem.Error()).To(ContainSubstring("OAS linting did not pass"))
		})
	})

	Context("when no ApiCategories exist in the cluster", func() {
		It("should skip linting entirely", func() {
			ml := &mockLinter{outcome: oaslint.Blocked, err: nil}
			ctrl = &ApiSpecificationController{
				Linter: ml,
			}

			apiSpec := &roverv1.ApiSpecification{
				Spec: roverv1.ApiSpecificationSpec{
					BasePath: "/test/api/v1",
					Category: "test-cat",
				},
			}

			// nil categoryList (store not configured)
			err := ctrl.checkAndLintSpec(context.Background(), apiSpec, nil, []byte("openapi: 3.0.0"))
			Expect(err).ToNot(HaveOccurred())
			Expect(ml.called).To(BeFalse(), "linter should not be called when categoryList is nil")

			// empty categoryList (no CRs in cluster)
			ml.called = false
			emptyList := &apiv1.ApiCategoryList{Items: []apiv1.ApiCategory{}}
			err = ctrl.checkAndLintSpec(context.Background(), apiSpec, emptyList, []byte("openapi: 3.0.0"))
			Expect(err).ToNot(HaveOccurred())
			Expect(ml.called).To(BeFalse(), "linter should not be called when no ApiCategories exist")
		})
	})
})
