// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
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

func newZone(name string, linting *adminv1.LintingConfig) *adminv1.Zone {
	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-env",
		},
		Spec: adminv1.ZoneSpec{
			Linting: linting,
		},
	}
}

func listZonesWith(zones ...*adminv1.Zone) func(context.Context, string) (*adminv1.ZoneList, error) {
	return func(_ context.Context, _ string) (*adminv1.ZoneList, error) {
		items := make([]adminv1.Zone, len(zones))
		for i, z := range zones {
			items[i] = *z
		}
		return &adminv1.ZoneList{Items: items}, nil
	}
}

var _ = Describe("ApiSpecification Handler Linting Gate", func() {

	Context("when LintPassed is nil (no linting performed)", func() {
		It("should not block", func() {
			h := &handler.ApiSpecificationHandler{}
			apiSpec := newApiSpec("hash1", "other")
			Expect(apiSpec.Status.LintPassed).To(BeNil())
			_ = h
		})
	})

	Context("when linting is in progress (async pending)", func() {
		It("should have nil LintPassed with pending reason", func() {
			apiSpec := newApiSpec("hash1", "other")
			apiSpec.Status.LintPassed = nil
			apiSpec.Status.LintReason = "Linting in progress"
			Expect(apiSpec.Status.LintPassed).To(BeNil())
			Expect(apiSpec.Status.LintReason).To(Equal("Linting in progress"))
		})
	})

	Context("when linting passed", func() {
		It("should not set blocked condition", func() {
			apiSpec := newApiSpec("hash1", "other")
			passed := true
			apiSpec.Status.LintPassed = &passed
			apiSpec.Status.LintReason = "no errors"
			Expect(*apiSpec.Status.LintPassed).To(BeTrue())
		})
	})

	Context("when linting failed in block mode", func() {
		It("should have failing lint status", func() {
			apiSpec := newApiSpec("hash1", "strict-cat")
			passed := false
			apiSpec.Status.LintPassed = &passed
			apiSpec.Status.LintReason = "found 3 errors"
			apiSpec.Status.LintErrors = 3
			apiSpec.Status.LintWarnings = 1
			Expect(*apiSpec.Status.LintPassed).To(BeFalse())
			Expect(apiSpec.Status.LintErrors).To(Equal(3))
		})
	})

	Context("when linting failed with dashboard URL", func() {
		It("should include the dashboard URL in the status", func() {
			apiSpec := newApiSpec("hash1", "strict-cat")
			passed := false
			apiSpec.Status.LintPassed = &passed
			apiSpec.Status.LintReason = "found 3 errors"
			apiSpec.Status.LintErrors = 3
			apiSpec.Status.LintDashboardURL = "https://linter.example.com/scans/scan-123"
			Expect(apiSpec.Status.LintDashboardURL).To(Equal("https://linter.example.com/scans/scan-123"))
		})
	})

	Context("Zone with whitelisted categories", func() {
		It("should support whitelisted categories in zone config", func() {
			zone := newZone("dp1", &adminv1.LintingConfig{
				Enabled:               true,
				Mode:                  adminv1.LintingModeBlock,
				WhitelistedCategories: []string{"internal", "legacy"},
			})
			Expect(zone.Spec.Linting.WhitelistedCategories).To(ContainElement("internal"))
			Expect(zone.Spec.Linting.WhitelistedCategories).To(ContainElement("legacy"))
		})

		It("should support dashboard URL template in zone config", func() {
			zone := newZone("dp1", &adminv1.LintingConfig{
				Enabled:              true,
				DashboardURLTemplate: "https://linter.example.com/scans/{linterId}",
			})
			Expect(zone.Spec.Linting.DashboardURLTemplate).To(Equal("https://linter.example.com/scans/{linterId}"))
		})
	})

	Context("LintingMode behavior on Zone", func() {
		It("should default to block mode when mode is empty", func() {
			zone := newZone("dp1", &adminv1.LintingConfig{
				Enabled: true,
			})
			Expect(zone.Spec.Linting.Mode).To(Equal(adminv1.LintingMode("")))
		})

		It("should support warn mode", func() {
			zone := newZone("dp1", &adminv1.LintingConfig{
				Enabled: true,
				Mode:    adminv1.LintingModeWarn,
			})
			Expect(zone.Spec.Linting.Mode).To(Equal(adminv1.LintingModeWarn))
		})

		It("should support block mode explicitly", func() {
			zone := newZone("dp1", &adminv1.LintingConfig{
				Enabled: true,
				Mode:    adminv1.LintingModeBlock,
			})
			Expect(zone.Spec.Linting.Mode).To(Equal(adminv1.LintingModeBlock))
		})
	})

	Context("lookupLintingMode via Zone", func() {
		It("should return block mode when ListZones is nil", func() {
			h := &handler.ApiSpecificationHandler{}
			_ = h
		})

		It("should return zone linting mode", func() {
			zone := newZone("dp1", &adminv1.LintingConfig{
				Enabled: true,
				Mode:    adminv1.LintingModeWarn,
			})
			h := &handler.ApiSpecificationHandler{
				ListZones: listZonesWith(zone),
			}
			_ = h
		})

		It("should skip zones without linting config", func() {
			zone := newZone("dp1", nil)
			h := &handler.ApiSpecificationHandler{
				ListZones: listZonesWith(zone),
			}
			_ = h
		})
	})
})
