// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak_test

import (
	"errors"
	"net/url"

	"github.com/telekom/controlplane/identity/pkg/keycloak"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DetermineIssuerUrlFrom", func() {
	DescribeTable("should replace only the admin path segment",
		func(adminUrl, realmName, expected string) {
			issuerUrl, err := keycloak.DetermineIssuerUrlFrom(adminUrl, realmName)
			Expect(err).ToNot(HaveOccurred())
			Expect(issuerUrl).To(Equal(expected))
		},
		Entry("context path", "https://iam.example.com/auth/admin/realms/", "my-realm", "https://iam.example.com/auth/realms/my-realm"),
		Entry("no context path", "https://iam.example.com/admin/realms/", "my-realm", "https://iam.example.com/realms/my-realm"),
		Entry("no trailing slash", "https://iam.example.com/auth/admin/realms", "my-realm", "https://iam.example.com/auth/realms/my-realm"),
		Entry("admin-prefixed host", "https://admin.example.com/auth/admin/realms/", "my-realm", "https://admin.example.com/auth/realms/my-realm"),
		Entry("admin host without path", "https://admin.example.com", "master", "https://admin.example.com/realms/master"),
		Entry("admin host with trailing slash", "https://admin.example.com/", "master", "https://admin.example.com/realms/master"),
		Entry("port", "http://localhost:8080/admin/realms/", "master", "http://localhost:8080/realms/master"),
		Entry("admin-prefixed path segment", "https://iam.example.com/administration/admin/realms/", "r", "https://iam.example.com/administration/realms/r"),
		Entry("escaped context path", "https://iam.example.com/a%2Fb/admin/realms/", "r", "https://iam.example.com/a%2Fb/realms/r"),
		Entry("pre-escaped realm name is kept", "https://iam.example.com/admin/realms/", "a%20b", "https://iam.example.com/realms/a%20b"),
		Entry("query is dropped", "https://iam.example.com/admin/realms/?x=1", "r", "https://iam.example.com/realms/r"),
	)

	It("should derive the admin token URL", func() {
		tokenUrl, err := keycloak.DetermineAdminTokenUrlFrom("https://admin.example.com/auth/admin/realms/", keycloak.MasterRealm)
		Expect(err).ToNot(HaveOccurred())
		Expect(tokenUrl).To(Equal("https://admin.example.com/auth/realms/master/protocol/openid-connect/token"))
	})

	DescribeTable("should reject a malformed admin URL without exposing it",
		func(determine func(adminUrl, realmName string) (string, error)) {
			result, err := determine("https://user:s3cret@admin.example.com:bad/auth/admin/realms/", "r")
			Expect(err).To(MatchError(ContainSubstring("invalid admin URL")))
			Expect(err.Error()).ToNot(ContainSubstring("s3cret"))
			Expect(err.Error()).ToNot(ContainSubstring("admin.example.com"))
			var urlErr *url.Error
			Expect(errors.As(err, &urlErr)).To(BeFalse())
			Expect(result).To(BeEmpty())
		},
		Entry("issuer URL", keycloak.DetermineIssuerUrlFrom),
		Entry("admin token URL", keycloak.DetermineAdminTokenUrlFrom),
	)
})
