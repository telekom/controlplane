// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Realm", func() {
	Describe("AsDownstreams", func() {
		It("should return single downstream for default realm", func() {
			realm := &gatewayv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: "test-zone",
				},
				Spec: gatewayv1.RealmSpec{
					Urls:       []string{"https://gateway.example.com/"},
					IssuerUrls: []string{"https://idp.example.com/auth/realms/default"},
				},
			}

			downstreams, err := realm.AsDownstreams("/api/v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(downstreams).To(HaveLen(1))
			Expect(downstreams[0].Host).To(Equal("gateway.example.com"))
			Expect(downstreams[0].Port).To(Equal(443))
			Expect(downstreams[0].Path).To(Equal("/api/v1"))
			Expect(downstreams[0].IssuerUrl).To(Equal("https://idp.example.com/auth/realms/default"))
		})

		It("should return multiple downstreams for DTC realm with 3 zones", func() {
			realm := &gatewayv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dtc",
					Namespace: "test-zone",
				},
				Spec: gatewayv1.RealmSpec{
					Urls: []string{
						"https://gateway-default.example.com/",
						"https://gateway-zone1.example.com/",
						"https://gateway-zone2.example.com/",
					},
					IssuerUrls: []string{
						"https://idp-default.example.com/auth/realms/dtc",
						"https://idp-zone1.example.com/auth/realms/dtc",
						"https://idp-zone2.example.com/auth/realms/dtc",
					},
				},
			}

			downstreams, err := realm.AsDownstreams("/api/v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(downstreams).To(HaveLen(3))

			// Verify first downstream (default zone)
			Expect(downstreams[0].Host).To(Equal("gateway-default.example.com"))
			Expect(downstreams[0].Port).To(Equal(443))
			Expect(downstreams[0].Path).To(Equal("/api/v1"))
			Expect(downstreams[0].IssuerUrl).To(Equal("https://idp-default.example.com/auth/realms/dtc"))

			// Verify second downstream (zone1)
			Expect(downstreams[1].Host).To(Equal("gateway-zone1.example.com"))
			Expect(downstreams[1].Port).To(Equal(443))
			Expect(downstreams[1].Path).To(Equal("/api/v1"))
			Expect(downstreams[1].IssuerUrl).To(Equal("https://idp-zone1.example.com/auth/realms/dtc"))

			// Verify third downstream (zone2)
			Expect(downstreams[2].Host).To(Equal("gateway-zone2.example.com"))
			Expect(downstreams[2].Port).To(Equal(443))
			Expect(downstreams[2].Path).To(Equal("/api/v1"))
			Expect(downstreams[2].IssuerUrl).To(Equal("https://idp-zone2.example.com/auth/realms/dtc"))
		})

		It("should handle mismatched URLs and issuers count (more URLs than issuers)", func() {
			realm := &gatewayv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dtc",
					Namespace: "test-zone",
				},
				Spec: gatewayv1.RealmSpec{
					Urls: []string{
						"https://gateway1.example.com/",
						"https://gateway2.example.com/",
						"https://gateway3.example.com/",
						"https://gateway4.example.com/",
					},
					IssuerUrls: []string{
						"https://idp1.example.com/auth/realms/dtc",
						"https://idp2.example.com/auth/realms/dtc",
					},
				},
			}

			downstreams, err := realm.AsDownstreams("/api/v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(downstreams).To(HaveLen(4))

			// First two downstreams should have matching issuers
			Expect(downstreams[0].IssuerUrl).To(Equal("https://idp1.example.com/auth/realms/dtc"))
			Expect(downstreams[1].IssuerUrl).To(Equal("https://idp2.example.com/auth/realms/dtc"))

			// Last two downstreams should have empty issuer URLs
			Expect(downstreams[2].IssuerUrl).To(BeEmpty())
			Expect(downstreams[3].IssuerUrl).To(BeEmpty())
		})

		It("should handle base path correctly with URL path", func() {
			realm := &gatewayv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: "test-zone",
				},
				Spec: gatewayv1.RealmSpec{
					Urls:       []string{"https://gateway.example.com/prefix/"},
					IssuerUrls: []string{"https://idp.example.com/auth/realms/default"},
				},
			}

			downstreams, err := realm.AsDownstreams("/api/v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(downstreams).To(HaveLen(1))
			Expect(downstreams[0].Path).To(Equal("/prefix/api/v1"))
		})

		It("should return error for realm with no URLs", func() {
			realm := &gatewayv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid",
					Namespace: "test-zone",
				},
				Spec: gatewayv1.RealmSpec{
					Urls:       []string{},
					IssuerUrls: []string{},
				},
			}

			_, err := realm.AsDownstreams("/api/v1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no downstreams found"))
		})

		It("should return error for invalid URL", func() {
			realm := &gatewayv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid",
					Namespace: "test-zone",
				},
				Spec: gatewayv1.RealmSpec{
					Urls:       []string{"://invalid-url"},
					IssuerUrls: []string{},
				},
			}

			_, err := realm.AsDownstreams("/api/v1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse URL"))
		})

		It("should handle HTTP URLs (non-HTTPS)", func() {
			realm := &gatewayv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-zone",
				},
				Spec: gatewayv1.RealmSpec{
					Urls:       []string{"http://gateway.example.com:8080/"},
					IssuerUrls: []string{"http://idp.example.com:8080/auth/realms/test"},
				},
			}

			downstreams, err := realm.AsDownstreams("/api/v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(downstreams).To(HaveLen(1))
			Expect(downstreams[0].Host).To(Equal("gateway.example.com"))
			Expect(downstreams[0].Port).To(Equal(8080))
			Expect(downstreams[0].Path).To(Equal("/api/v1"))
		})
	})
})
