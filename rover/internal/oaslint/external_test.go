// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package oaslint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExternalLinter", func() {
	var (
		ctx    context.Context
		server *httptest.Server
		linter *ExternalLinter
		spec   []byte
	)

	BeforeEach(func() {
		ctx = context.Background()
		spec = []byte(`openapi: "3.0.0"
info:
  title: Test API
  version: "1.0.0"
servers:
  - url: http://example.com/api/v1
`)
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Context("when the linter API returns a clean scan", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.URL.Path).To(Equal("/api/linter/scans"))
				Expect(r.Header.Get("Content-Type")).To(Equal(yamlContentType))

				resp := linterScanResponse{
					ID:        "scan-123",
					CreatedAt: "2025-01-01T00:00:00Z",
					Ruleset: linterRuleset{
						Name: "default-ruleset",
						Hash: "abc123",
					},
					Info: violationsInfo{
						Infos:    1,
						Warnings: 2,
						Errors:   0,
						Hints:    3,
					},
					LinterVersion: "1.5.0",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp) //nolint:errcheck
			}))
			linter = NewExternalLinter(server.URL)
		})

		It("should return a passing result", func() {
			result, err := linter.Lint(ctx, spec, "default-ruleset")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Passed).To(BeTrue())
			Expect(result.LinterId).To(Equal("scan-123"))
			Expect(result.Ruleset).To(Equal("default-ruleset"))
			Expect(result.LinterVersion).To(Equal("1.5.0"))
			Expect(result.Errors).To(Equal(0))
			Expect(result.Warnings).To(Equal(2))
			Expect(result.Hints).To(Equal(3))
			Expect(result.Infos).To(Equal(1))
			Expect(result.Reason).To(ContainSubstring("does not contain errors"))
		})
	})

	Context("when the linter API returns errors", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				resp := linterScanResponse{
					ID: "scan-456",
					Ruleset: linterRuleset{
						Name: "strict-ruleset",
					},
					Info: violationsInfo{
						Errors:   5,
						Warnings: 3,
					},
					LinterVersion: "1.5.0",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp) //nolint:errcheck
			}))
			linter = NewExternalLinter(server.URL)
		})

		It("should return a failing result", func() {
			result, err := linter.Lint(ctx, spec, "strict-ruleset")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Passed).To(BeFalse())
			Expect(result.Errors).To(Equal(5))
			Expect(result.Warnings).To(Equal(3))
			Expect(result.LinterId).To(Equal("scan-456"))
			Expect(result.Reason).To(ContainSubstring("5 error(s)"))
			Expect(result.Reason).To(ContainSubstring("strict-ruleset"))
		})
	})

	Context("when the linter API returns 5xx", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			linter = NewExternalLinter(server.URL)
		})

		It("should return an error", func() {
			result, err := linter.Lint(ctx, spec, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("linter service unavailable"))
			Expect(result).To(BeNil())
		})
	})

	Context("when the linter API returns 408 timeout", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusRequestTimeout)
			}))
			linter = NewExternalLinter(server.URL)
		})

		It("should return a timeout error", func() {
			result, err := linter.Lint(ctx, spec, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timed out"))
			Expect(result).To(BeNil())
		})
	})

	Context("when the linter API is unreachable", func() {
		BeforeEach(func() {
			linter = NewExternalLinter("http://localhost:1", WithHTTPClient(&http.Client{
				Timeout: 1 * time.Second,
			}))
		})

		It("should return a connection error", func() {
			result, err := linter.Lint(ctx, spec, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("calling linter API"))
			Expect(result).To(BeNil())
		})
	})

	Context("when the linter API returns invalid JSON", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("not json")) //nolint:errcheck
			}))
			linter = NewExternalLinter(server.URL)
		})

		It("should return a decode error", func() {
			result, err := linter.Lint(ctx, spec, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("decoding linter response"))
			Expect(result).To(BeNil())
		})
	})
})

var _ = Describe("NoopLinter", func() {
	It("should always return a passing result", func() {
		linter := &NoopLinter{}
		result, err := linter.Lint(context.Background(), []byte("anything"), "any-ruleset")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Passed).To(BeTrue())
		Expect(result.Reason).To(ContainSubstring("disabled"))
	})
})
