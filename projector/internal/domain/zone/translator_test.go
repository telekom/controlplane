// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone_test

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/domain/zone"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Zone Translator", func() {
	var t *zone.Translator

	BeforeEach(func() {
		t = &zone.Translator{}
	})

	Describe("ShouldSkip", func() {
		It("always returns false", func() {
			obj := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{Name: "zone-a"},
			}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		DescribeTable("maps Zone CR to ZoneData",
			func(obj *adminv1.Zone, expected *zone.ZoneData) {
				result, err := t.Translate(context.Background(), obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expected))
			},
			Entry("world visibility with gateway URL",
				&adminv1.Zone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "zone-a",
						Namespace: "admin",
						Labels: map[string]string{
							"cp.ei.telekom.de/environment": "production",
						},
					},
					Spec: adminv1.ZoneSpec{
						Visibility: adminv1.ZoneVisibilityWorld,
						Gateway: adminv1.GatewayConfig{
							Url: "https://gateway.example.com",
						},
					},
				},
				&zone.ZoneData{
					Meta:       shared.NewMetadata("admin", "zone-a", map[string]string{"cp.ei.telekom.de/environment": "production"}),
					Name:       "zone-a",
					GatewayURL: strPtr("https://gateway.example.com"),
					Visibility: "WORLD",
				},
			),
			Entry("enterprise visibility without gateway URL",
				&adminv1.Zone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "zone-b",
						Namespace: "admin",
						Labels: map[string]string{
							"cp.ei.telekom.de/environment": "staging",
						},
					},
					Spec: adminv1.ZoneSpec{
						Visibility: adminv1.ZoneVisibilityEnterprise,
						Gateway:    adminv1.GatewayConfig{},
					},
				},
				&zone.ZoneData{
					Meta:       shared.NewMetadata("admin", "zone-b", map[string]string{"cp.ei.telekom.de/environment": "staging"}),
					Name:       "zone-b",
					GatewayURL: nil,
					Visibility: "ENTERPRISE",
				},
			),
			Entry("no labels",
				&adminv1.Zone{
					ObjectMeta: metav1.ObjectMeta{
						Name: "zone-c",
					},
					Spec: adminv1.ZoneSpec{
						Visibility: adminv1.ZoneVisibilityWorld,
						Gateway: adminv1.GatewayConfig{
							Url: "https://gw.test",
						},
					},
				},
				&zone.ZoneData{
					Meta:       shared.NewMetadata("", "zone-c", nil),
					Name:       "zone-c",
					GatewayURL: strPtr("https://gw.test"),
					Visibility: "WORLD",
				},
			),
			Entry("empty gateway URL is treated as nil",
				&adminv1.Zone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "zone-d",
						Namespace: "admin",
					},
					Spec: adminv1.ZoneSpec{
						Visibility: adminv1.ZoneVisibilityEnterprise,
						Gateway: adminv1.GatewayConfig{
							Url: "",
						},
					},
				},
				&zone.ZoneData{
					Meta:       shared.NewMetadata("admin", "zone-d", nil),
					Name:       "zone-d",
					GatewayURL: nil,
					Visibility: "ENTERPRISE",
				},
			),
		)
	})

	Describe("KeyFromObject", func() {
		It("returns ZoneKey from object name", func() {
			obj := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{Name: "test-zone"},
			}
			Expect(t.KeyFromObject(obj)).To(Equal(zone.ZoneKey("test-zone")))
		})
	})

	Describe("KeyFromDelete", func() {
		It("derives key from request name (Strong strategy)", func() {
			req := types.NamespacedName{Name: "deleted-zone", Namespace: "admin"}
			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key).To(Equal(zone.ZoneKey("deleted-zone")))
		})

		It("ignores lastKnown even when provided", func() {
			req := types.NamespacedName{Name: "deleted-zone"}
			lastKnown := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{Name: "different-name"},
			}
			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key).To(Equal(zone.ZoneKey("deleted-zone")))
		})
	})
})

func strPtr(s string) *string {
	return &s
}
