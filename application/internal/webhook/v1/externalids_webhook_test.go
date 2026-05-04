// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApplicationWebhook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Application Webhook Suite")
}

const (
	testEnv = "test-env"
	testNs  = "test-env"
)

var _ = Describe("Application ExternalIds webhook validation", func() {
	var (
		ctx       context.Context
		validator *ApplicationCustomValidator
	)

	makeApp := func(zoneName string, extIds []applicationv1.ExternalId) *applicationv1.Application {
		return &applicationv1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: applicationv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "apps",
				Labels: map[string]string{
					config.EnvironmentLabelKey: testEnv,
				},
			},
			Spec: applicationv1.ApplicationSpec{
				Team:        "team-a",
				TeamEmail:   "team-a@example.com",
				Secret:      "secret",
				Zone:        ctypes.ObjectRef{Name: zoneName, Namespace: testNs},
				ExternalIds: extIds,
			},
		}
	}

	makeZone := func(name string, policies []adminv1.ExternalIdPolicy) *adminv1.Zone {
		return &adminv1.Zone{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Zone",
				APIVersion: adminv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNs,
				Labels:    map[string]string{config.EnvironmentLabelKey: testEnv},
			},
			Spec: adminv1.ZoneSpec{
				Visibility:         adminv1.ZoneVisibilityWorld,
				ExternalIdPolicies: policies,
			},
		}
	}

	BeforeEach(func() {
		ctx = context.Background()

		Expect(adminv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(applicationv1.AddToScheme(scheme.Scheme)).To(Succeed())

		fc := fake.NewClientBuilder().WithScheme(scheme.Scheme).
			WithObjects(
				makeZone("zone-enforced", []adminv1.ExternalIdPolicy{
					{Scheme: "psi", Required: true, Pattern: `^PSI-[0-9]{6}$`},
					{Scheme: "icto", Required: false, Pattern: `^icto-[0-9]+$`},
				}),
				makeZone("zone-open", nil),
			).
			Build()
		validator = &ApplicationCustomValidator{client: fc}
	})

	It("rejects a missing required scheme", func() {
		app := makeApp("zone-enforced", nil)
		_, err := validator.ValidateCreate(ctx, app)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`scheme "psi" is required`))
	})

	It("accepts a matching required externalId", func() {
		app := makeApp("zone-enforced", []applicationv1.ExternalId{
			{Scheme: "psi", Id: "PSI-103596"},
		})
		_, err := validator.ValidateCreate(ctx, app)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects a required scheme with a bad id", func() {
		app := makeApp("zone-enforced", []applicationv1.ExternalId{
			{Scheme: "psi", Id: "PSI-BAD"},
		})
		_, err := validator.ValidateCreate(ctx, app)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`^PSI-[0-9]{6}$`))
	})

	It("rejects a non-required scheme whose id fails the pattern", func() {
		app := makeApp("zone-enforced", []applicationv1.ExternalId{
			{Scheme: "psi", Id: "PSI-103596"},
			{Scheme: "icto", Id: "not-an-icto"},
		})
		_, err := validator.ValidateCreate(ctx, app)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`^icto-[0-9]+$`))
	})

	It("ignores schemes with no matching zone policy", func() {
		app := makeApp("zone-enforced", []applicationv1.ExternalId{
			{Scheme: "psi", Id: "PSI-103596"},
			{Scheme: "unknown", Id: "whatever"},
		})
		_, err := validator.ValidateCreate(ctx, app)
		Expect(err).NotTo(HaveOccurred())
	})

	It("accepts any externalIds when the zone has no policies", func() {
		app := makeApp("zone-open", []applicationv1.ExternalId{
			{Scheme: "psi", Id: "literally-anything"},
		})
		_, err := validator.ValidateCreate(ctx, app)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects when the referenced zone does not exist", func() {
		app := makeApp("nonexistent-zone", nil)
		_, err := validator.ValidateCreate(ctx, app)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`zone 'nonexistent-zone' not found`))
	})
})
