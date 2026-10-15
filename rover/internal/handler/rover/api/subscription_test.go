// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	apiapi "github.com/telekom/controlplane/api/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("mapSubscriberSecurityToApiSecurity", func() {
	DescribeTable("passes the consumer grant type through", func(grantType rover.GrantType, expected apiapi.GrantType) {
		mapped := mapSubscriberSecurityToApiSecurity(&rover.SubscriberSecurity{M2M: &rover.SubscriberMachine2MachineAuthentication{
			Scopes:    []string{"read"},
			GrantType: grantType,
		}})

		Expect(mapped).NotTo(BeNil())
		Expect(mapped.M2M).NotTo(BeNil())
		Expect(mapped.M2M.GrantType).To(Equal(expected))
	},
		Entry("empty without defaulting", rover.GrantType(""), apiapi.GrantType("")),
		Entry("password", rover.GrantTypePassword, apiapi.GrantTypePassword),
		Entry("client_credentials", rover.GrantTypeClientCredentials, apiapi.GrantTypeClientCredentials),
		Entry("authorization_code", rover.GrantTypeAuthorizationCode, apiapi.GrantTypeAuthorizationCode),
		Entry("refresh_token", rover.GrantTypeRefreshToken, apiapi.GrantTypeRefreshToken),
		// CRD enum validation rejects unrecognized grant types. This direct mapper test
		// bypasses API-server validation and checks pass-through behavior only.
		Entry("unrecognized without validation", rover.GrantType("unrecognized"), apiapi.GrantType("unrecognized")),
	)

	DescribeTable("passes the consumer token request method through", func(tokenRequest rover.TokenRequestMethod, expected apiapi.TokenRequestMethod) {
		mapped := mapSubscriberSecurityToApiSecurity(&rover.SubscriberSecurity{M2M: &rover.SubscriberMachine2MachineAuthentication{
			Client:       &rover.OAuth2ClientCredentials{ClientId: "consumer-id", ClientSecret: "consumer-secret"},
			TokenRequest: tokenRequest,
		}})

		Expect(mapped).NotTo(BeNil())
		Expect(mapped.M2M).NotTo(BeNil())
		Expect(mapped.M2M.TokenRequest).To(Equal(expected))
	},
		Entry("empty without defaulting", rover.TokenRequestMethod(""), apiapi.TokenRequestMethod("")),
		Entry("client_secret_basic", rover.TokenRequestClientSecretBasic, apiapi.TokenRequestClientSecretBasic),
		Entry("client_secret_post", rover.TokenRequestClientSecretPost, apiapi.TokenRequestClientSecretPost),
	)
})
