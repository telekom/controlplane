// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

var _ = Describe("ValidateRealmStatus", func() {

	It("should return an error when status is nil", func() {
		err := ValidateRealmStatus(nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("realmStatus is nil"))
	})

	It("should return an error when IssuerUrl is empty", func() {
		err := ValidateRealmStatus(&identityv1.RealmStatus{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("realmStatus.IssuerUrl is empty"))
	})

	It("should return an error when AdminClientId is empty", func() {
		err := ValidateRealmStatus(&identityv1.RealmStatus{
			IssuerUrl: "https://issuer.example.com",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("realmStatus.AdminClientId is empty"))
	})

	It("should return an error when AdminUserName is empty", func() {
		err := ValidateRealmStatus(&identityv1.RealmStatus{
			IssuerUrl:     "https://issuer.example.com",
			AdminClientId: "admin-client-id",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("realmStatus.AdminUserName is empty"))
	})

	It("should return an error when AdminPassword is empty", func() {
		err := ValidateRealmStatus(&identityv1.RealmStatus{
			IssuerUrl:     "https://issuer.example.com",
			AdminClientId: "admin-client-id",
			AdminUserName: "admin-username",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("realmStatus.AdminPassword is empty"))
	})

	It("should return an error when AdminUrl is empty", func() {
		err := ValidateRealmStatus(&identityv1.RealmStatus{
			IssuerUrl:     "https://issuer.example.com",
			AdminClientId: "admin-client-id",
			AdminUserName: "admin-username",
			AdminPassword: "admin-password",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("realmStatus.AdminUrl is empty"))
	})

	It("should return an error when AdminTokenUrl is empty", func() {
		err := ValidateRealmStatus(&identityv1.RealmStatus{
			IssuerUrl:     "https://issuer.example.com",
			AdminClientId: "admin-client-id",
			AdminUserName: "admin-username",
			AdminPassword: "admin-password",
			AdminUrl:      "https://admin.example.com",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("realmStatus.AdminTokenUrl is empty"))
	})

	It("should return nil when all fields are valid", func() {
		err := ValidateRealmStatus(&identityv1.RealmStatus{
			IssuerUrl:     "https://issuer.example.com",
			AdminClientId: "admin-client-id",
			AdminUserName: "admin-username",
			AdminPassword: "admin-password",
			AdminUrl:      "https://admin.example.com",
			AdminTokenUrl: "https://admin.example.com/token",
		})
		Expect(err).ToNot(HaveOccurred())
	})
})
