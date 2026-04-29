// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Linting helpers", func() {
	Describe("isBasepathWhitelisted", func() {
		It("should return false when WhitelistedBasepaths is empty", func() {
			cfg := &apiv1.LintingConfig{}
			Expect(isBasepathWhitelisted(cfg, "/eni/test/v1")).To(BeFalse())
		})

		It("should return true for exact match", func() {
			cfg := &apiv1.LintingConfig{
				WhitelistedBasepaths: []string{"/eni/test/v1"},
			}
			Expect(isBasepathWhitelisted(cfg, "/eni/test/v1")).To(BeTrue())
		})

		It("should match case-insensitively", func() {
			cfg := &apiv1.LintingConfig{
				WhitelistedBasepaths: []string{"/ENI/Test/v1"},
			}
			Expect(isBasepathWhitelisted(cfg, "/eni/test/v1")).To(BeTrue())
		})

		It("should return false for non-matching basepath", func() {
			cfg := &apiv1.LintingConfig{
				WhitelistedBasepaths: []string{"/other/path/v1"},
			}
			Expect(isBasepathWhitelisted(cfg, "/eni/test/v1")).To(BeFalse())
		})

		It("should check all entries", func() {
			cfg := &apiv1.LintingConfig{
				WhitelistedBasepaths: []string{"/first/v1", "/second/v2", "/eni/test/v1"},
			}
			Expect(isBasepathWhitelisted(cfg, "/eni/test/v1")).To(BeTrue())
		})
	})

	Describe("prepareLinting", func() {
		var ctrl *ApiSpecificationController

		BeforeEach(func() {
			ctrl = &ApiSpecificationController{}
		})

		It("should skip linting for category-whitelisted basepath", func() {
			lintCfg := &apiv1.LintingConfig{
				URL:                  "https://linter.example.com",
				WhitelistedBasepaths: []string{"/eni/internal/v1"},
			}
			apiSpec := &roverv1.ApiSpecification{
				Spec: roverv1.ApiSpecificationSpec{BasePath: "/eni/internal/v1"},
			}

			result := ctrl.prepareLinting(lintCfg, apiSpec, nil)
			Expect(result).To(BeFalse())
			Expect(apiSpec.Spec.Lint).ToNot(BeNil())
			Expect(apiSpec.Spec.Lint.Passed).To(BeTrue())
			Expect(apiSpec.Spec.Lint.Message).To(ContainSubstring("whitelisted"))
		})

		It("should skip linting when spec hash is unchanged and previous result exists", func() {
			lintCfg := &apiv1.LintingConfig{URL: "https://linter.example.com"}
			existing := &roverv1.ApiSpecification{
				Spec: roverv1.ApiSpecificationSpec{
					BasePath: "/eni/test/v1",
					Hash:     "same-hash",
					Lint:     &roverv1.LintResult{Passed: true, Message: "all good"},
				},
			}
			apiSpec := &roverv1.ApiSpecification{
				Spec: roverv1.ApiSpecificationSpec{
					BasePath: "/eni/test/v1",
					Hash:     "same-hash",
				},
			}

			result := ctrl.prepareLinting(lintCfg, apiSpec, existing)
			Expect(result).To(BeFalse())
			Expect(apiSpec.Spec.Lint).ToNot(BeNil())
			Expect(apiSpec.Spec.Lint.Passed).To(BeTrue())
		})

		It("should require linting when spec hash changed", func() {
			lintCfg := &apiv1.LintingConfig{URL: "https://linter.example.com"}
			existing := &roverv1.ApiSpecification{
				Spec: roverv1.ApiSpecificationSpec{
					BasePath: "/eni/test/v1",
					Hash:     "old-hash",
					Lint:     &roverv1.LintResult{Passed: true, Message: "all good"},
				},
			}
			apiSpec := &roverv1.ApiSpecification{
				Spec: roverv1.ApiSpecificationSpec{
					BasePath: "/eni/test/v1",
					Hash:     "new-hash",
				},
			}

			result := ctrl.prepareLinting(lintCfg, apiSpec, existing)
			Expect(result).To(BeTrue())
			Expect(apiSpec.Spec.Lint).To(BeNil())
		})

		It("should require linting for new spec (no existing object)", func() {
			lintCfg := &apiv1.LintingConfig{URL: "https://linter.example.com"}
			apiSpec := &roverv1.ApiSpecification{
				Spec: roverv1.ApiSpecificationSpec{
					BasePath: "/eni/test/v1",
					Hash:     "brand-new-hash",
				},
			}

			result := ctrl.prepareLinting(lintCfg, apiSpec, nil)
			Expect(result).To(BeTrue())
			Expect(apiSpec.Spec.Lint).To(BeNil())
		})
	})

	Describe("lookupLintingConfig", func() {
		It("should return nil when ListApiCategories is nil", func() {
			ctrl := &ApiSpecificationController{}
			result := ctrl.lookupLintingConfig(context.Background(), "some-cat")
			Expect(result).To(BeNil())
		})

		It("should return nil when category is not found", func() {
			ctrl := &ApiSpecificationController{
				ListApiCategories: func(_ context.Context) (*apiv1.ApiCategoryList, error) {
					return &apiv1.ApiCategoryList{Items: []apiv1.ApiCategory{}}, nil
				},
			}
			result := ctrl.lookupLintingConfig(context.Background(), "nonexistent")
			Expect(result).To(BeNil())
		})

		It("should return linting config from matching category", func() {
			ctrl := &ApiSpecificationController{
				ListApiCategories: func(_ context.Context) (*apiv1.ApiCategoryList, error) {
					return &apiv1.ApiCategoryList{
						Items: []apiv1.ApiCategory{
							{
								ObjectMeta: metav1.ObjectMeta{Name: "my-cat"},
								Spec: apiv1.ApiCategorySpec{
									LabelValue: "my-cat",
									Linting: &apiv1.LintingConfig{
										URL:  "https://linter.example.com",
										Mode: apiv1.LintingModeWarn,
									},
								},
							},
						},
					}, nil
				},
			}
			result := ctrl.lookupLintingConfig(context.Background(), "my-cat")
			Expect(result).ToNot(BeNil())
			Expect(result.URL).To(Equal("https://linter.example.com"))
			Expect(result.Mode).To(Equal(apiv1.LintingModeWarn))
		})

		It("should return nil when category has no linting config", func() {
			ctrl := &ApiSpecificationController{
				ListApiCategories: func(_ context.Context) (*apiv1.ApiCategoryList, error) {
					return &apiv1.ApiCategoryList{
						Items: []apiv1.ApiCategory{
							{
								ObjectMeta: metav1.ObjectMeta{Name: "no-lint-cat"},
								Spec: apiv1.ApiCategorySpec{
									LabelValue: "no-lint-cat",
								},
							},
						},
					}, nil
				},
			}
			result := ctrl.lookupLintingConfig(context.Background(), "no-lint-cat")
			Expect(result).To(BeNil())
		})

		It("should return nil when ListApiCategories returns error", func() {
			ctrl := &ApiSpecificationController{
				ListApiCategories: func(_ context.Context) (*apiv1.ApiCategoryList, error) {
					return nil, fmt.Errorf("store error")
				},
			}
			result := ctrl.lookupLintingConfig(context.Background(), "some-cat")
			Expect(result).To(BeNil())
		})
	})
})
