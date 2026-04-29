// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	handler "github.com/telekom/controlplane/rover/internal/handler/apispecification"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newApiSpec(hash, category string) *roverv1.ApiSpecification {
	return &roverv1.ApiSpecification{
		ObjectMeta: metav1.ObjectMeta{
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

var _ = Describe("ApiSpecification Handler Linting Gate", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("when linting is pending (Spec.Lint nil, block mode)", func() {
		It("should set processing and not-ready conditions", func() {
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryWith(newApiCategory("other", &apiapi.LintingConfig{
					Mode: apiapi.LintingModeBlock,
				})),
			}
			apiSpec := newApiSpec("hash1", "other")
			// Spec.Lint is nil — linting pending

			err := h.CreateOrUpdate(ctx, apiSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeTrue())
			Expect(hasCondition(apiSpec, condition.ConditionTypeReady)).To(BeTrue())
			Expect(conditionMessage(apiSpec, condition.ConditionTypeProcessing)).To(ContainSubstring("linting is in progress"))
			Expect(conditionMessage(apiSpec, condition.ConditionTypeReady)).To(ContainSubstring("being linted"))
		})
	})

	Context("when linting is pending (Spec.Lint nil, warn mode)", func() {
		It("should proceed with Api creation", func() {
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryWith(newApiCategory("warn-cat", &apiapi.LintingConfig{
					Mode: apiapi.LintingModeWarn,
				})),
			}
			apiSpec := newApiSpec("hash1", "warn-cat")
			// Spec.Lint is nil — linting pending, but warn mode proceeds

			Expect(func() {
				_ = h.CreateOrUpdate(ctx, apiSpec)
			}).To(Panic())
			// Panicked in createOrUpdateApi means the linting gate did not block.
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeFalse())
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
		It("should not set blocked condition", func() {
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryWith(newApiCategory("warn-cat", &apiapi.LintingConfig{
					Mode: apiapi.LintingModeWarn,
				})),
			}
			apiSpec := newApiSpec("hash1", "warn-cat")
			apiSpec.Spec.Lint = &roverv1.LintResult{Passed: false, Message: "found 2 warnings"}

			// CreateOrUpdate will proceed to createOrUpdateApi which requires a k8s client;
			// we expect it to panic or error there, but the linting gate should NOT block.
			Expect(func() {
				_ = h.CreateOrUpdate(ctx, apiSpec)
			}).To(Panic())
			// If we got here (panicked in createOrUpdateApi), it means the linting gate passed through.
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeFalse(),
				"should not have a blocked/processing condition in warn mode")
		})
	})

	Context("when linting passed", func() {
		It("should proceed past linting gate", func() {
			h := &handler.ApiSpecificationHandler{}
			apiSpec := newApiSpec("hash1", "other")
			apiSpec.Spec.Lint = &roverv1.LintResult{Passed: true, Message: "no errors"}

			// Will proceed to createOrUpdateApi -> panic on missing k8s client
			Expect(func() {
				_ = h.CreateOrUpdate(ctx, apiSpec)
			}).To(Panic())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeFalse())
		})
	})

	Context("when no linting is configured (Spec.Lint nil, no category linting)", func() {
		It("should proceed when GetApiCategory is nil", func() {
			h := &handler.ApiSpecificationHandler{}
			apiSpec := newApiSpec("hash1", "other")

			Expect(func() {
				_ = h.CreateOrUpdate(ctx, apiSpec)
			}).To(Panic())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeFalse())
		})

		It("should proceed when category has no linting config", func() {
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryNil(),
			}
			apiSpec := newApiSpec("hash1", "other")

			Expect(func() {
				_ = h.CreateOrUpdate(ctx, apiSpec)
			}).To(Panic())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeFalse())
		})

		It("should proceed when category lookup returns error", func() {
			h := &handler.ApiSpecificationHandler{
				GetApiCategory: getApiCategoryError(),
			}
			apiSpec := newApiSpec("hash1", "other")

			Expect(func() {
				_ = h.CreateOrUpdate(ctx, apiSpec)
			}).To(Panic())
			Expect(hasCondition(apiSpec, condition.ConditionTypeProcessing)).To(BeFalse())
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
