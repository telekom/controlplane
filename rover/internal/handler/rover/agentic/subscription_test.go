// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agentic

import (
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("mapSubscriberSecurityToAgenticSecurity", func() {
	DescribeTable("passes the consumer grant type through", func(grantType rover.GrantType, expected agenticv1.GrantType) {
		mapped := mapSubscriberSecurityToAgenticSecurity(&rover.SubscriberSecurity{M2M: &rover.SubscriberMachine2MachineAuthentication{
			Scopes:    []string{"read"},
			GrantType: grantType,
		}})

		Expect(mapped).NotTo(BeNil())
		Expect(mapped.M2M).NotTo(BeNil())
		Expect(mapped.M2M.GrantType).To(Equal(expected))
	},
		Entry("empty without defaulting", rover.GrantType(""), agenticv1.GrantType("")),
		Entry("password", rover.GrantTypePassword, agenticv1.GrantTypePassword),
		Entry("client_credentials", rover.GrantTypeClientCredentials, agenticv1.GrantTypeClientCredentials),
		Entry("authorization_code", rover.GrantTypeAuthorizationCode, agenticv1.GrantTypeAuthorizationCode),
		Entry("refresh_token without validation", rover.GrantTypeRefreshToken, agenticv1.GrantType("refresh_token")),
		Entry("unrecognized without validation", rover.GrantType("unrecognized"), agenticv1.GrantType("unrecognized")),
	)

	DescribeTable("passes the consumer token request method through", func(tokenRequest rover.TokenRequestMethod, expected agenticv1.TokenRequestMethod) {
		mapped := mapSubscriberSecurityToAgenticSecurity(&rover.SubscriberSecurity{M2M: &rover.SubscriberMachine2MachineAuthentication{
			Client:       &rover.OAuth2ClientCredentials{ClientId: "consumer-id", ClientSecret: "consumer-secret"},
			TokenRequest: tokenRequest,
		}})

		Expect(mapped).NotTo(BeNil())
		Expect(mapped.M2M).NotTo(BeNil())
		Expect(mapped.M2M.TokenRequest).To(Equal(expected))
	},
		Entry("empty without defaulting", rover.TokenRequestMethod(""), agenticv1.TokenRequestMethod("")),
		Entry("client_secret_basic", rover.TokenRequestClientSecretBasic, agenticv1.TokenRequestClientSecretBasic),
		Entry("client_secret_post", rover.TokenRequestClientSecretPost, agenticv1.TokenRequestClientSecretPost),
	)
})
