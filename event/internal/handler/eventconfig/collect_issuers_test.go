// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventconfig

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func zoneWithIssuers(name, issuer, lmsIssuer string) *adminv1.Zone {
	z := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: adminv1.ZoneSpec{Presets: []adminv1.Preset{
			{Name: "api", Type: adminv1.GatewayTypeAPI, Default: true},
			{Name: "event", Type: adminv1.GatewayTypeEvent, Default: true},
		}},
		// Event routing reads the Event preset, so the issuers under test live there.
		Status: adminv1.ZoneStatus{Presets: []adminv1.PresetStatus{
			{Name: "api"},
			{Name: "event", Links: adminv1.Links{Issuer: issuer, LmsIssuer: lmsIssuer}},
		}},
	}
	return z
}

var _ = Describe("collectPrimaryTrustedIssuers", func() {
	DescribeTable("collects issuers",
		func(myZone *adminv1.Zone, otherZones []*adminv1.Zone, isProxyTarget bool, expected []string) {
			issuers, err := collectPrimaryTrustedIssuers(myZone, otherZones, isProxyTarget)
			Expect(err).NotTo(HaveOccurred())
			Expect(issuers).To(Equal(expected))
		},
		Entry("own issuer only", zoneWithIssuers("zone-a", "idp-a", "lms-a"), []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", "lms-b")}, false, []string{"idp-a"}),
		Entry("empty own issuer", zoneWithIssuers("zone-a", "", "lms-a"), nil, false, nil),
		Entry("peer LMS issuers", zoneWithIssuers("zone-a", "idp-a", "lms-a"), []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", "lms-b"), zoneWithIssuers("zone-c", "idp-c", "lms-c")}, true, []string{"idp-a", "lms-b", "lms-c"}),
		Entry("empty peer LMS issuer", zoneWithIssuers("zone-a", "idp-a", "lms-a"), []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", ""), zoneWithIssuers("zone-c", "idp-c", "lms-c")}, true, []string{"idp-a", "lms-c"}),
	)

	It("uses the Event preset status rather than the API one", func() {
		zone := zoneWithIssuers("zone-a", "event-idp", "event-lms")
		apiStatus, err := zone.Status.GetPreset("api")
		Expect(err).NotTo(HaveOccurred())
		apiStatus.Links.Issuer = "wrong-idp"

		issuers, err := collectPrimaryTrustedIssuers(zone, nil, false)

		Expect(err).NotTo(HaveOccurred())
		Expect(issuers).To(Equal([]string{"event-idp"}))
	})

	It("returns an error when the Event preset status is missing", func() {
		zone := zoneWithIssuers("zone-a", "event-idp", "event-lms")
		zone.Status.Presets = nil

		issuers, err := collectPrimaryTrustedIssuers(zone, nil, false)

		Expect(err).To(HaveOccurred())
		Expect(issuers).To(BeNil())
	})
})
