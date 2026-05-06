// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	handler "github.com/telekom/controlplane/rover/internal/handler/apispecification"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newApiSpec(hash, category string) *roverv1.ApiSpecification {
	return &roverv1.ApiSpecification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-spec",
			Namespace: "test-env--test-team",
			UID:       "test-uid-1234",
			Labels: map[string]string{
				"controlplane.2/environment": "test-env",
			},
		},
		Spec: roverv1.ApiSpecificationSpec{
			Specification: "file-id-123",
			Category:      category,
			BasePath:      "/eni/test/v1",
			Hash:          hash,
			Version:       "1.0.0",
		},
	}
}

func newApiCategory(name string, linting *apiapi.LintingConfig) *apiapi.ApiCategory {
	return &apiapi.ApiCategory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-env",
			Labels: map[string]string{
				"controlplane.2/label": name,
			},
		},
		Spec: apiapi.ApiCategorySpec{
			Linting: linting,
		},
	}
}

func getApiCategoryWith(cat *apiapi.ApiCategory) func(context.Context, string) (*apiapi.ApiCategory, error) {
	return func(_ context.Context, _ string) (*apiapi.ApiCategory, error) {
		return cat, nil
	}
}

func getApiCategoryNil() func(context.Context, string) (*apiapi.ApiCategory, error) {
	return func(_ context.Context, _ string) (*apiapi.ApiCategory, error) {
		return nil, nil
	}
}

func getApiCategoryError() func(context.Context, string) (*apiapi.ApiCategory, error) {
	return func(_ context.Context, _ string) (*apiapi.ApiCategory, error) {
		return nil, fmt.Errorf("api category lookup failed")
	}
}

func hasCondition(apiSpec *roverv1.ApiSpecification, condType string) bool {
	for _, c := range apiSpec.GetConditions() {
		if c.Type == condType {
			return true
		}
	}
	return false
}

func conditionMessage(apiSpec *roverv1.ApiSpecification, condType string) string {
	for _, c := range apiSpec.GetConditions() {
		if c.Type == condType {
			return c.Message
		}
	}
	return ""
}

// setupMockClient creates a mock JanitorClient injected into context.
// The mock expects CreateOrUpdate and returns success.
func setupMockClient(ctx context.Context) context.Context {
	fakeClient := fakeclient.NewMockJanitorClient(GinkgoT())
	testScheme := runtime.NewScheme()
	_ = roverv1.AddToScheme(testScheme)
	_ = apiapi.AddToScheme(testScheme)

	fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
	fakeClient.EXPECT().
		CreateOrUpdate(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ client.Object, fn controllerutil.MutateFn) (controllerutil.OperationResult, error) {
			_ = fn()
			return controllerutil.OperationResultCreated, nil
		}).Maybe()
	fakeClient.EXPECT().AnyChanged().Return(true).Maybe()

	return cclient.WithClient(ctx, fakeClient)
}

var _ = Describe("ApiSpecification Handler Linting Gate", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("when linting is pending (Spec.Lint nil, block mode)", func() {
		It("should set not-ready condition", func() {
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryWith(newApiCategory("other", &apiapi.LintingConfig{
					Mode: apiapi.LintingModeBlock,
				})),
			}
			apiSpec := newApiSpec("hash1", "other")

			err := h.CreateOrUpdate(ctx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeReady)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeReady)).To(ContainSubstring("being linted"))
		})
	})

	Context("when linting is pending (Spec.Lint nil, warn mode)", func() {
		It("should proceed with Api creation", func() {
			mockCtx := setupMockClient(ctx)
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryWith(newApiCategory("warn-cat", &apiapi.LintingConfig{
					Mode: apiapi.LintingModeWarn,
				})),
			}
			apiSpec := newApiSpec("hash1", "warn-cat")

			err := h.CreateOrUpdate(mockCtx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("API updated"))
		})
	})

	Context("when linting failed in block mode", func() {
		It("should set blocked condition with explicit block mode", func() {
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryWith(newApiCategory("strict-cat", &apiapi.LintingConfig{
					Mode: apiapi.LintingModeBlock,
				})),
			}
			apiSpec := newApiSpec("hash1", "strict-cat")
			apiSpec.Spec.Lint = &roverv1.LintResult{Passed: false, Message: "found 3 errors"}

			err := h.CreateOrUpdate(ctx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("found 3 errors"))
		})

		It("should set blocked condition with dashboard URL", func() {
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryWith(newApiCategory("strict-cat", &apiapi.LintingConfig{
					Mode: apiapi.LintingModeBlock,
				})),
			}
			apiSpec := newApiSpec("hash1", "strict-cat")
			apiSpec.Spec.Lint = &roverv1.LintResult{
				Passed:       false,
				Message:      "found 3 errors",
				DashboardURL: "https://linter.example.com/scans/scan-123",
			}

			err := h.CreateOrUpdate(ctx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("View details"))
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("scan-123"))
		})

		It("should default to block mode when linting mode is empty string", func() {
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryWith(newApiCategory("test-cat", &apiapi.LintingConfig{
					Mode: "",
				})),
			}
			apiSpec := newApiSpec("hash1", "test-cat")
			apiSpec.Spec.Lint = &roverv1.LintResult{Passed: false, Message: "found errors"}

			err := h.CreateOrUpdate(ctx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
		})
	})

	Context("when linting failed in warn mode", func() {
		It("should proceed with Api creation", func() {
			mockCtx := setupMockClient(ctx)
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryWith(newApiCategory("warn-cat", &apiapi.LintingConfig{
					Mode: apiapi.LintingModeWarn,
				})),
			}
			apiSpec := newApiSpec("hash1", "warn-cat")
			apiSpec.Spec.Lint = &roverv1.LintResult{Passed: false, Message: "found 2 warnings"}

			err := h.CreateOrUpdate(mockCtx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("API updated"))
		})
	})

	Context("when linting passed", func() {
		It("should proceed with Api creation", func() {
			mockCtx := setupMockClient(ctx)
			h := &handler.ApiSpecificationHandler{}
			apiSpec := newApiSpec("hash1", "other")
			apiSpec.Spec.Lint = &roverv1.LintResult{Passed: true, Message: "no errors"}

			err := h.CreateOrUpdate(mockCtx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("API updated"))
		})
	})

	Context("when no linting is configured (Spec.Lint nil, no category linting)", func() {
		It("should proceed when GetApiCategory is nil", func() {
			mockCtx := setupMockClient(ctx)
			h := &handler.ApiSpecificationHandler{}
			apiSpec := newApiSpec("hash1", "other")

			err := h.CreateOrUpdate(mockCtx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("API updated"))
		})

		It("should proceed when category has no linting config", func() {
			mockCtx := setupMockClient(ctx)
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryNil(),
			}
			apiSpec := newApiSpec("hash1", "other")

			err := h.CreateOrUpdate(mockCtx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("API updated"))
		})

		It("should proceed when category lookup returns error", func() {
			mockCtx := setupMockClient(ctx)
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryError(),
			}
			apiSpec := newApiSpec("hash1", "other")

			err := h.CreateOrUpdate(mockCtx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("API updated"))
		})
	})

	Context("Delete", func() {
		It("should return nil", func() {
			h := &handler.ApiSpecificationHandler{}
			err := h.Delete(ctx, newApiSpec("hash1", "other"))
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
