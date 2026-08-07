// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func zoneWithIssuers(name, issuer, lmsIssuer string) *adminv1.Zone {
	z := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       adminv1.ZoneSpec{Presets: []adminv1.Preset{{Name: "default", Default: true}}},
		Status:     adminv1.ZoneStatus{Presets: []adminv1.PresetStatus{{Name: "default"}}},
	}
	z.Status.Presets[0].Links.Issuer = issuer
	z.Status.Presets[0].Links.LmsIssuer = lmsIssuer
	return z
}

var _ = Describe("collectPrimaryTrustedIssuers", func() {
	DescribeTable("collects issuers",
		func(zone *adminv1.Zone, subscriberZones []*adminv1.Zone, isProxyTarget bool, expected []string) {
			issuers, err := collectPrimaryTrustedIssuers(zone, subscriberZones, isProxyTarget)
			Expect(err).NotTo(HaveOccurred())
			Expect(issuers).To(Equal(expected))
		},
		Entry("own issuer only", zoneWithIssuers("zone-a", "idp-a", "lms-a"), []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", "lms-b")}, false, []string{"idp-a"}),
		Entry("empty own issuer", zoneWithIssuers("zone-a", "", "lms-a"), nil, false, nil),
		Entry("subscriber LMS issuers", zoneWithIssuers("zone-a", "idp-a", "lms-a"), []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", "lms-b"), zoneWithIssuers("zone-c", "idp-c", "lms-c")}, true, []string{"idp-a", "lms-b", "lms-c"}),
		Entry("empty subscriber LMS issuer", zoneWithIssuers("zone-a", "idp-a", "lms-a"), []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", ""), zoneWithIssuers("zone-c", "idp-c", "lms-c")}, true, []string{"idp-a", "lms-c"}),
	)

	It("uses the matching default preset status rather than the first status", func() {
		zone := zoneWithIssuers("zone-a", "default-idp", "default-lms")
		zone.Spec.Presets = append([]adminv1.Preset{{Name: "alpha"}}, zone.Spec.Presets...)
		zone.Status.Presets = append([]adminv1.PresetStatus{{Name: "alpha", Links: adminv1.Links{Issuer: "wrong-idp"}}}, zone.Status.Presets...)

		issuers, err := collectPrimaryTrustedIssuers(zone, nil, false)

		Expect(err).NotTo(HaveOccurred())
		Expect(issuers).To(Equal([]string{"default-idp"}))
	})

	It("returns an error when the default preset status is missing", func() {
		zone := zoneWithIssuers("zone-a", "default-idp", "default-lms")
		zone.Status.Presets = nil

		issuers, err := collectPrimaryTrustedIssuers(zone, nil, false)

		Expect(err).To(HaveOccurred())
		Expect(issuers).To(BeNil())
	})
})
