// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

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
		var linter *apiLinterImpl

		BeforeEach(func() {
			linter = &apiLinterImpl{}
		})

		It("should skip linting for category-whitelisted basepath", func() {
			lintCfg := &apiv1.LintingConfig{
				WhitelistedBasepaths: []string{"/eni/internal/v1"},
			}
			apiSpec := &roverv1.ApiSpecification{
				Spec: roverv1.ApiSpecificationSpec{BasePath: "/eni/internal/v1"},
			}

			result := linter.prepareLinting(lintCfg, apiSpec)
			Expect(result).To(BeFalse())
			Expect(apiSpec.Spec.Lint).ToNot(BeNil())
			Expect(apiSpec.Spec.Lint.Passed).To(BeTrue())
			Expect(apiSpec.Spec.Lint.Message).To(ContainSubstring("whitelisted"))
		})

		It("should require linting when basepath is not whitelisted", func() {
			lintCfg := &apiv1.LintingConfig{}
			apiSpec := &roverv1.ApiSpecification{
				Spec: roverv1.ApiSpecificationSpec{BasePath: "/eni/test/v1"},
			}

			result := linter.prepareLinting(lintCfg, apiSpec)
			Expect(result).To(BeTrue())
			Expect(apiSpec.Spec.Lint).To(BeNil())
		})
	})

	Describe("Lint", func() {
		var (
			lintCtx      context.Context
			linterServer *httptest.Server
			linter       ApiLinter
			apiSpec      *roverv1.ApiSpecification
			category     *apiv1.ApiCategory
			specBytes    []byte
		)

		newCategory := func(mode apiv1.LintingMode) *apiv1.ApiCategory {
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

		startLinterServer := func(errors int) *httptest.Server {
			return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				resp := map[string]any{
					"id":            "scan-test",
					"createdAt":     "2025-01-01T00:00:00Z",
					"ruleset":       map[string]any{"name": "default", "hash": "abc"},
					"info":          map[string]any{"errors": errors, "warnings": 0, "infos": 0, "hints": 0},
					"linterVersion": "1.0.0",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
			}))
		}

		BeforeEach(func() {
			lintCtx = context.Background()
			specBytes = []byte("openapi: '3.0.0'")
			apiSpec = &roverv1.ApiSpecification{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-spec",
					Namespace: "env--ns",
				},
				Spec: roverv1.ApiSpecificationSpec{
					BasePath: "/test/api/v1",
					Category: "test-cat",
					Hash:     "new-hash",
				},
			}
		})

		AfterEach(func() {
			if linterServer != nil {
				linterServer.Close()
			}
		})

		It("should skip when category is nil", func() {
			linter = &apiLinterImpl{url: "http://linter"}
			outcome, err := linter.Lint(lintCtx, apiSpec, nil, specBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(outcome).To(Equal(LintSkipped))
		})

		It("should skip when mode is None", func() {
			linter = &apiLinterImpl{url: "http://linter"}
			category = newCategory(apiv1.LintingModeNone)
			outcome, err := linter.Lint(lintCtx, apiSpec, category, specBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(outcome).To(Equal(LintSkipped))
		})

		It("should skip when linter URL is empty", func() {
			linter = &apiLinterImpl{url: ""}
			category = newCategory(apiv1.LintingModeWarn)
			outcome, err := linter.Lint(lintCtx, apiSpec, category, specBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(outcome).To(Equal(LintSkipped))
		})

		Context("when linting passes", func() {
			BeforeEach(func() {
				linterServer = startLinterServer(0)
			})

			It("should return LintCompleted in Block mode", func() {
				linter = &apiLinterImpl{url: linterServer.URL, httpClient: linterServer.Client()}
				category = newCategory(apiv1.LintingModeBlock)
				outcome, err := linter.Lint(lintCtx, apiSpec, category, specBytes)
				Expect(err).ToNot(HaveOccurred())
				Expect(outcome).To(Equal(LintCompleted))
				Expect(apiSpec.Spec.Lint).ToNot(BeNil())
				Expect(apiSpec.Spec.Lint.Passed).To(BeTrue())
			})

			It("should return LintCompleted in Warn mode", func() {
				linter = &apiLinterImpl{url: linterServer.URL, httpClient: linterServer.Client()}
				category = newCategory(apiv1.LintingModeWarn)
				outcome, err := linter.Lint(lintCtx, apiSpec, category, specBytes)
				Expect(err).ToNot(HaveOccurred())
				Expect(outcome).To(Equal(LintCompleted))
				Expect(apiSpec.Spec.Lint).ToNot(BeNil())
				Expect(apiSpec.Spec.Lint.Passed).To(BeTrue())
			})
		})

		Context("when linting fails (spec has errors)", func() {
			BeforeEach(func() {
				linterServer = startLinterServer(3)
			})

			It("should return LintBlocked with error in Block mode", func() {
				linter = &apiLinterImpl{url: linterServer.URL, httpClient: linterServer.Client()}
				category = newCategory(apiv1.LintingModeBlock)
				outcome, err := linter.Lint(lintCtx, apiSpec, category, specBytes)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("linting failed in block mode"))
				Expect(outcome).To(Equal(LintBlocked))
				Expect(apiSpec.Spec.Lint).ToNot(BeNil())
				Expect(apiSpec.Spec.Lint.Passed).To(BeFalse())
			})

			It("should return LintCompleted without error in Warn mode", func() {
				linter = &apiLinterImpl{url: linterServer.URL, httpClient: linterServer.Client()}
				category = newCategory(apiv1.LintingModeWarn)
				outcome, err := linter.Lint(lintCtx, apiSpec, category, specBytes)
				Expect(err).ToNot(HaveOccurred())
				Expect(outcome).To(Equal(LintCompleted))
				Expect(apiSpec.Spec.Lint).ToNot(BeNil())
				Expect(apiSpec.Spec.Lint.Passed).To(BeFalse())
			})
		})

		Context("when linter API is unreachable", func() {
			It("should return error without persisting", func() {
				linter = &apiLinterImpl{url: "http://localhost:1", httpClient: &http.Client{}}
				category = newCategory(apiv1.LintingModeBlock)
				outcome, err := linter.Lint(lintCtx, apiSpec, category, specBytes)
				Expect(err).To(HaveOccurred())
				Expect(outcome).To(Equal(LintCompleted))
			})
		})
	})
})

// mockLinter is a simple test double for ApiLinter that records whether Lint was called.
type mockLinter struct {
	called  bool
	outcome LintOutcome
	err     error
}

func (m *mockLinter) Lint(_ context.Context, apiSpec *roverv1.ApiSpecification, _ *apiv1.ApiCategory, _ []byte) (LintOutcome, error) {
	m.called = true
	if apiSpec.Spec.Lint == nil {
		apiSpec.Spec.Lint = &roverv1.LintResult{Passed: true, Message: "mock lint ran"}
	}
	return m.outcome, m.err
}

var _ = Describe("lintOrReuse (hash dedup)", func() {
	var (
		ctrl      *ApiSpecificationController
		linterMck *mockLinter
		apiSpec   *roverv1.ApiSpecification
		specBytes []byte
	)

	BeforeEach(func() {
		linterMck = &mockLinter{outcome: LintCompleted}
		ctrl = &ApiSpecificationController{Linter: linterMck}
		specBytes = []byte("openapi: '3.0.0'")
		apiSpec = &roverv1.ApiSpecification{
			Spec: roverv1.ApiSpecificationSpec{
				Hash: "new-hash",
			},
		}
	})

	It("should call Lint when there is no existing spec", func() {
		outcome, err := ctrl.lintOrReuse(context.Background(), apiSpec, nil, nil, specBytes)
		Expect(err).ToNot(HaveOccurred())
		Expect(outcome).To(Equal(LintCompleted))
		Expect(linterMck.called).To(BeTrue())
	})

	It("should call Lint when existing has no lint result", func() {
		existing := &roverv1.ApiSpecification{
			Spec: roverv1.ApiSpecificationSpec{Hash: "new-hash"},
		}
		outcome, err := ctrl.lintOrReuse(context.Background(), apiSpec, existing, nil, specBytes)
		Expect(err).ToNot(HaveOccurred())
		Expect(outcome).To(Equal(LintCompleted))
		Expect(linterMck.called).To(BeTrue())
	})

	It("should call Lint when hash changed", func() {
		existing := &roverv1.ApiSpecification{
			Spec: roverv1.ApiSpecificationSpec{
				Hash: "old-hash",
				Lint: &roverv1.LintResult{Passed: true, Message: "old result"},
			},
		}
		outcome, err := ctrl.lintOrReuse(context.Background(), apiSpec, existing, nil, specBytes)
		Expect(err).ToNot(HaveOccurred())
		Expect(outcome).To(Equal(LintCompleted))
		Expect(linterMck.called).To(BeTrue())
	})

	It("should reuse cached result when hash is unchanged", func() {
		existing := &roverv1.ApiSpecification{
			Spec: roverv1.ApiSpecificationSpec{
				Hash: "new-hash",
				Lint: &roverv1.LintResult{Passed: true, Message: "cached"},
			},
		}
		outcome, err := ctrl.lintOrReuse(context.Background(), apiSpec, existing, nil, specBytes)
		Expect(err).ToNot(HaveOccurred())
		Expect(outcome).To(Equal(LintSkipped))
		Expect(linterMck.called).To(BeFalse())
		Expect(apiSpec.Spec.Lint).ToNot(BeNil())
		Expect(apiSpec.Spec.Lint.Passed).To(BeTrue())
		Expect(apiSpec.Spec.Lint.Message).To(Equal("cached"))
	})

	It("should reuse cached failure result when hash is unchanged", func() {
		existing := &roverv1.ApiSpecification{
			Spec: roverv1.ApiSpecificationSpec{
				Hash: "new-hash",
				Lint: &roverv1.LintResult{Passed: false, Message: "previous failure"},
			},
		}
		outcome, err := ctrl.lintOrReuse(context.Background(), apiSpec, existing, nil, specBytes)
		Expect(err).ToNot(HaveOccurred())
		Expect(outcome).To(Equal(LintSkipped))
		Expect(linterMck.called).To(BeFalse())
		Expect(apiSpec.Spec.Lint.Passed).To(BeFalse())
		Expect(apiSpec.Spec.Lint.Message).To(Equal("previous failure"))
	})
})
