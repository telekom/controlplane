// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("OpenAPI v3 client-credentials scope extraction", func() {
	DescribeTable("parses only supported flow scopes", func(schemes string, expected []string) {
		document := fmt.Sprintf(`{
			"openapi": "3.0.3",
			"info": {"title": "Test API", "version": "1.0.0"},
			"servers": [{"url": "https://example.com/test/v1"}],
			"paths": {},
			"components": {"securitySchemes": %s}
		}`, schemes)
		spec, err := ParseSpecification(context.Background(), document)
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.Spec.Oauth2Scopes).To(Equal(expected))
	},
		Entry("password only", `{"oauth": {"type": "oauth2", "flows": {"password": {"tokenUrl": "https://example.com/token", "scopes": {"ignored": ""}}}}}`, []string{}),
		Entry("authorization code only", `{"oauth": {"type": "oauth2", "flows": {"authorizationCode": {"authorizationUrl": "https://example.com/authorize", "tokenUrl": "https://example.com/token", "scopes": {"ignored": ""}}}}}`, []string{}),
		Entry("implicit only", `{"oauth": {"type": "oauth2", "flows": {"implicit": {"authorizationUrl": "https://example.com/authorize", "scopes": {"ignored": ""}}}}}`, []string{}),
		Entry("client credentials only preserves scope order", `{"oauth": {"type": "oauth2", "flows": {"clientCredentials": {"tokenUrl": "https://example.com/token", "scopes": {"write": "", "read": ""}}}}}`, []string{"write", "read"}),
		Entry("mixed schemes visits supported flows after skipped schemes", `{
			"bearer": {"type": "http", "scheme": "bearer"},
			"user": {"type": "oauth2", "flows": {"password": {"tokenUrl": "https://example.com/token", "scopes": {"ignored": ""}}}},
			"service": {"type": "oauth2", "flows": {"clientCredentials": {"tokenUrl": "https://example.com/token", "scopes": {"read": ""}}}}
		}`, []string{"read"}),
		Entry("mixed flows within one scheme", `{"oauth": {"type": "oauth2", "flows": {
			"authorizationCode": {"authorizationUrl": "https://example.com/authorize", "tokenUrl": "https://example.com/token", "scopes": {"ignored": ""}},
			"clientCredentials": {"tokenUrl": "https://example.com/token", "scopes": {"read": ""}}
		}}}`, []string{"read"}),
		Entry("empty client-credentials scopes alongside another scheme", `{
			"service": {"type": "oauth2", "flows": {"clientCredentials": {"tokenUrl": "https://example.com/token", "scopes": {}}}},
			"user": {"type": "oauth2", "flows": {"authorizationCode": {"authorizationUrl": "https://example.com/authorize", "tokenUrl": "https://example.com/token", "scopes": {"ignored": ""}}}}
		}`, []string{}),
	)

	It("handles an absent scheme map", func() {
		spec := &roverv1.ApiSpecificationSpec{}
		setSecuritySchemeValues(spec, nil)
		Expect(spec.Oauth2Scopes).To(BeEmpty())
	})

	DescribeTable("skips absent extraction data and continues iteration", func(scheme *v3.SecurityScheme) {
		schemes := orderedmap.New[string, *v3.SecurityScheme]()
		schemes.Set("skipped", scheme)
		scopes := orderedmap.New[string, string]()
		scopes.Set("read", "")
		schemes.Set("supported", &v3.SecurityScheme{
			Type: "oauth2",
			Flows: &v3.OAuthFlows{
				ClientCredentials: &v3.OAuthFlow{Scopes: scopes},
			},
		})
		spec := &roverv1.ApiSpecificationSpec{}
		setSecuritySchemeValues(spec, schemes)
		Expect(spec.Oauth2Scopes).To(Equal([]string{"read"}))
	},
		Entry("nil scheme", (*v3.SecurityScheme)(nil)),
		Entry("non-OAuth2 scheme", &v3.SecurityScheme{Type: "http"}),
		Entry("absent flows", &v3.SecurityScheme{Type: "oauth2"}),
		Entry("absent client-credentials flow", &v3.SecurityScheme{Type: "oauth2", Flows: &v3.OAuthFlows{}}),
		Entry("absent scopes", &v3.SecurityScheme{Type: "oauth2", Flows: &v3.OAuthFlows{ClientCredentials: &v3.OAuthFlow{}}}),
		Entry("empty scopes", &v3.SecurityScheme{Type: "oauth2", Flows: &v3.OAuthFlows{ClientCredentials: &v3.OAuthFlow{Scopes: orderedmap.New[string, string]()}}}),
	)
})
