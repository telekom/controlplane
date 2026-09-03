// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//nolint:unparam // params can be used in the future for more complex validation logic
package v1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/handler/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newValidZoneServiceConfig(name, namespace string) *filev1.ZoneServiceConfig {
	return &filev1.ZoneServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: "test-env",
			},
		},
		Spec: filev1.ZoneServiceConfigSpec{
			API: adminv1.ManagedRouteConfig{
				Name: "test-api",
				Path: "/api/v1",
				Url:  "http://test-api:8080",
				Type: adminv1.ManagedRouteTypeTeamAPI,
			},
			Zone: &types.ObjectRef{
				Name:      name,
				Namespace: strings.Split(namespace, "--")[0],
			},
		},
	}
}

func newValidZone(name, namespace string) *adminv1.Zone {
	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: adminv1.ZoneSpec{
			IdentityProvider: adminv1.IdentityProviderConfig{},
			Gateway:          adminv1.GatewayConfig{},
			Visibility:       adminv1.ZoneVisibilityWorld,
		},
	}
}

var _ = Describe("ZoneServiceConfig Webhook Validator", func() {
	Describe("ValidateCreate", func() {
		It("accepts a valid ZoneServiceConfig with a matching Zone", func() {
			zone := newValidZone("test-zone", "default")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(zone).Build()

			validator := &ZoneServiceConfigValidator{client: fakeClient}
			cfg := newValidZoneServiceConfig("test-zone", util.GetZoneNamespace(types.ObjectRefFromObject(zone)))

			_, err := validator.ValidateCreate(ctx, cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects a ZoneServiceConfig when no matching Zone exists", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

			validator := &ZoneServiceConfigValidator{client: fakeClient}
			cfg := newValidZoneServiceConfig("nonexistent-zone", "default")

			_, err := validator.ValidateCreate(ctx, cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Zone with name"))
		})

		It("rejects a ZoneServiceConfig when the name does not match the Zone", func() {
			zone := newValidZone("test-zone", "default")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(zone).Build()

			validator := &ZoneServiceConfigValidator{client: fakeClient}
			cfg := newValidZoneServiceConfig("mismatched-name", util.GetZoneNamespace(types.ObjectRefFromObject(zone)))

			_, err := validator.ValidateCreate(ctx, cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must match the Zone name and it must be in the namespace of the Zone it configures"))
		})

		It("rejects a ZoneServiceConfig when the namespace does not match the Zone", func() {
			zone := newValidZone("test-zone", "default")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(zone).Build()

			validator := &ZoneServiceConfigValidator{client: fakeClient}
			cfg := newValidZoneServiceConfig(zone.Name, "wrong--namespace")

			_, err := validator.ValidateCreate(ctx, cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must match the Zone name and it must be in the namespace of the Zone it configures"))
		})

		It("accepts with valid service endpoint configuration", func() {
			zone := newValidZone("test-zone-ep", "default")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(zone).Build()

			validator := &ZoneServiceConfigValidator{client: fakeClient}
			cfg := newValidZoneServiceConfig("test-zone-ep", util.GetZoneNamespace(types.ObjectRefFromObject((zone))))
			cfg.Spec.Service = &filev1.ServiceEndpoint{
				Host: "sftp.example.com",
				Port: 22,
			}

			_, err := validator.ValidateCreate(ctx, cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("does not reject empty host (validated by CRD)", func() {
			zone := newValidZone("test-zone-bad-host", "default")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(zone).Build()

			validator := &ZoneServiceConfigValidator{client: fakeClient}
			cfg := newValidZoneServiceConfig("test-zone-bad-host", util.GetZoneNamespace(types.ObjectRefFromObject(zone)))
			cfg.Spec.Service = &filev1.ServiceEndpoint{
				Host: "",
				Port: 22,
			}

			_, err := validator.ValidateCreate(ctx, cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("does not reject out-of-range port (validated by CRD)", func() {
			zone := newValidZone("test-zone-bad-port", "default")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(zone).Build()

			validator := &ZoneServiceConfigValidator{client: fakeClient}
			cfg := newValidZoneServiceConfig("test-zone-bad-port", util.GetZoneNamespace(types.ObjectRefFromObject(zone)))
			cfg.Spec.Service = &filev1.ServiceEndpoint{
				Host: "sftp.example.com",
				Port: 99999,
			}

			_, err := validator.ValidateCreate(ctx, cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("accepts valid IP address in service endpoint", func() {
			zone := newValidZone("test-zone-ip", "default")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(zone).Build()

			validator := &ZoneServiceConfigValidator{client: fakeClient}
			cfg := newValidZoneServiceConfig("test-zone-ip", util.GetZoneNamespace(types.ObjectRefFromObject(zone)))
			cfg.Spec.Service = &filev1.ServiceEndpoint{
				Host: "192.168.1.100",
				Port: 22,
			}

			_, err := validator.ValidateCreate(ctx, cfg)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ValidateUpdate", func() {
		It("accepts a valid update", func() {
			zone := newValidZone("update-zone", "default")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(zone).Build()

			validator := &ZoneServiceConfigValidator{client: fakeClient}
			cfg := newValidZoneServiceConfig("update-zone", util.GetZoneNamespace(types.ObjectRefFromObject(zone)))

			_, err := validator.ValidateUpdate(ctx, cfg, cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects an update when Zone no longer exists", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

			validator := &ZoneServiceConfigValidator{client: fakeClient}
			cfg := newValidZoneServiceConfig("missing-zone-update", "default")

			_, err := validator.ValidateUpdate(ctx, cfg, cfg)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ValidateDelete", func() {
		It("accepts deletion", func() {
			validator := &ZoneServiceConfigValidator{client: nil}
			cfg := newValidZoneServiceConfig("test-zone", "default")

			_, err := validator.ValidateDelete(ctx, cfg)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
