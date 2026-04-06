// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/rover/internal/handler/rover/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("GetDtcEligibleZones", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
		env    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		env = "test-env"

		// Create scheme with admin API types
		scheme = runtime.NewScheme()
		Expect(adminv1.AddToScheme(scheme)).To(Succeed())
	})

	It("should return zones with DTC URL and Enterprise visibility", func() {
		zone1 := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone1",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "https://dtc1.example.com/",
				},
				Visibility: adminv1.ZoneVisibilityEnterprise,
			},
		}

		zone2 := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone2",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "https://dtc2.example.com/",
				},
				Visibility: adminv1.ZoneVisibilityEnterprise,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(zone1, zone2).
			Build()

		scopedClient := client.NewScopedClient(fakeClient, env)
		c := client.NewJanitorClient(scopedClient)

		dtcZones, err := api.GetDtcEligibleZones(ctx, c, env)
		Expect(err).NotTo(HaveOccurred())
		Expect(dtcZones).To(HaveLen(2))
		Expect(dtcZones).To(ContainElements(
			types.ObjectRef{Name: "zone1", Namespace: env},
			types.ObjectRef{Name: "zone2", Namespace: env},
		))
	})

	It("should exclude zones without DTC URL", func() {
		zone1 := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone1",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "https://dtc1.example.com/",
				},
				Visibility: adminv1.ZoneVisibilityEnterprise,
			},
		}

		zone2WithoutDtc := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone2-no-dtc",
				Namespace: env,
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "", // No DTC URL
				},
				Visibility: adminv1.ZoneVisibilityEnterprise,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(zone1, zone2WithoutDtc).
			Build()

		scopedClient := client.NewScopedClient(fakeClient, env)
		c := client.NewJanitorClient(scopedClient)

		dtcZones, err := api.GetDtcEligibleZones(ctx, c, env)
		Expect(err).NotTo(HaveOccurred())
		Expect(dtcZones).To(HaveLen(1))
		Expect(dtcZones[0].Name).To(Equal("zone1"))
	})

	It("should exclude zones with World visibility", func() {
		zoneEnterprise := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone-enterprise",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "https://dtc-enterprise.example.com/",
				},
				Visibility: adminv1.ZoneVisibilityEnterprise,
			},
		}

		zoneWorld := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone-world",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "https://dtc-world.example.com/",
				},
				Visibility: adminv1.ZoneVisibilityWorld,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(zoneEnterprise, zoneWorld).
			Build()

		scopedClient := client.NewScopedClient(fakeClient, env)
		c := client.NewJanitorClient(scopedClient)

		dtcZones, err := api.GetDtcEligibleZones(ctx, c, env)
		Expect(err).NotTo(HaveOccurred())
		Expect(dtcZones).To(HaveLen(1))
		Expect(dtcZones[0].Name).To(Equal("zone-enterprise"))
	})

	It("should exclude zones without DTC URL even with Enterprise visibility", func() {
		zoneEnterprise := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone-enterprise",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "https://dtc-enterprise.example.com/",
				},
				Visibility: adminv1.ZoneVisibilityEnterprise,
			},
		}

		zoneLocal := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone-local",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "", // No DTC URL to simulate a local-only zone
				},
				Visibility: adminv1.ZoneVisibilityEnterprise,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(zoneEnterprise, zoneLocal).
			Build()

		scopedClient := client.NewScopedClient(fakeClient, env)
		c := client.NewJanitorClient(scopedClient)

		dtcZones, err := api.GetDtcEligibleZones(ctx, c, env)
		Expect(err).NotTo(HaveOccurred())
		Expect(dtcZones).To(HaveLen(1))
		Expect(dtcZones[0].Name).To(Equal("zone-enterprise"))
	})

	It("should return empty list when no zones are DTC-eligible", func() {
		zone1 := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone1",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "", // No DTC URL
				},
				Visibility: adminv1.ZoneVisibilityEnterprise,
			},
		}

		zone2 := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone2",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "https://dtc.example.com/",
				},
				Visibility: adminv1.ZoneVisibilityWorld, // Wrong visibility
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(zone1, zone2).
			Build()

		scopedClient := client.NewScopedClient(fakeClient, env)
		c := client.NewJanitorClient(scopedClient)

		dtcZones, err := api.GetDtcEligibleZones(ctx, c, env)
		Expect(err).NotTo(HaveOccurred())
		Expect(dtcZones).To(BeEmpty())
	})

	It("should return empty list when no zones exist", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		scopedClient := client.NewScopedClient(fakeClient, env)
		c := client.NewJanitorClient(scopedClient)

		dtcZones, err := api.GetDtcEligibleZones(ctx, c, env)
		Expect(err).NotTo(HaveOccurred())
		Expect(dtcZones).To(BeEmpty())
	})

	It("should handle mixed scenarios correctly", func() {
		// Zone with DTC + Enterprise (should be included)
		zone1 := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone-dtc-enterprise",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "https://dtc1.example.com/",
				},
				Visibility: adminv1.ZoneVisibilityEnterprise,
			},
		}

		// Zone with DTC + World (should be excluded)
		zone2 := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone-dtc-world",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "https://dtc2.example.com/",
				},
				Visibility: adminv1.ZoneVisibilityWorld,
			},
		}

		// Zone without DTC + Enterprise (should be excluded)
		zone3 := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone-no-dtc-enterprise",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "",
				},
				Visibility: adminv1.ZoneVisibilityEnterprise,
			},
		}

		// Zone with DTC + Enterprise (should be included)
		zone4 := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "zone-dtc-enterprise-2",
				Namespace: env,
				Labels: map[string]string{
					config.EnvironmentLabelKey: env,
				},
			},
			Spec: adminv1.ZoneSpec{
				Gateway: adminv1.GatewayConfig{
					DtcUrl: "https://dtc4.example.com/",
				},
				Visibility: adminv1.ZoneVisibilityEnterprise,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(zone1, zone2, zone3, zone4).
			Build()

		scopedClient := client.NewScopedClient(fakeClient, env)
		c := client.NewJanitorClient(scopedClient)

		dtcZones, err := api.GetDtcEligibleZones(ctx, c, env)
		Expect(err).NotTo(HaveOccurred())
		Expect(dtcZones).To(HaveLen(2))
		Expect(dtcZones).To(ContainElements(
			types.ObjectRef{Name: "zone-dtc-enterprise", Namespace: env},
			types.ObjectRef{Name: "zone-dtc-enterprise-2", Namespace: env},
		))
	})
})
