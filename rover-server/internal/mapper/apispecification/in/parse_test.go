// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("Apispecification Parser", func() {

	ctx := context.Background()
	specV2 := `
swagger: "2.0"
info:
  version: "1.0.0"
  title: "Test API"
  x-api-category: "test"
  x-vendor: "true"
basePath: "/eni/foo/v1"
securityDefinitions:
    oAuth2:
      type: oauth2
      description: dummy oauth2
      flow: clientCredentials
      scopes:
        read: read dummy
        write: write dummy
        admin: admin dummy
`

	specV3_0 := `
openapi: "3.0.0"
info:
  version: "1.0.0"
  title: "Test API"
  x-api-category: "test"
  x-vendor: "true"
servers:
- url: "https://example.com/eni/foo/v1"
components:
  securitySchemes:
    oAuth2:
      type: oauth2
      description: dummy oauth2
      flows:
        clientCredentials:
          tokenUrl: >-
            http://localhost:8080/proxy/auth/realms/default/protocol/openid-connect/token
          scopes:
            read: read dummy
            write: write dummy
            admin: admin dummy	
`

	specV3_0_without_basepath := `
openapi: "3.0.0"
info:
  version: "1.0.0"
  title: "Test API"
  x-api-category: "test"
  x-vendor: "true"
servers:
- url: "https://example.com"
components:
  securitySchemes:
    oAuth2:
      type: oauth2
      description: dummy oauth2
      flows:
        clientCredentials:
          tokenUrl: >-
            http://localhost:8080/proxy/auth/realms/default/protocol/openid-connect/token
          scopes:
            read: read dummy
            write: write dummy
            admin: admin dummy	
`

	specV3_1 := `
openapi: "3.1.0"
info:
  version: "1.0.0"
  title: "Test API"
  x-api-category: "test"
  x-vendor: "true"
servers:
- url: "https://example.com/eni/foo/v1"
components:
  securitySchemes:
    oAuth2:
      type: oauth2
      description: dummy oauth2
      flows:
        clientCredentials:
          tokenUrl: >-
            http://localhost:8080/proxy/auth/realms/default/protocol/openid-connect/token
          scopes:
            read: read dummy
            write: write dummy
            admin: admin dummy
`

	specNoExtraFields := `
openapi: "3.0.0"
info:
  version: "1.0.0"
  title: "Test API"
servers:
- url: "https://example.com/eni/foo/v1"	
`

	Context("When parsing a specification", func() {

		It("should correctly calculate them name", func() {
			expected := "ecc-pi-product-inventory-management-service-tmf-api-productinventory-v4"
			apiSpecification := &roverv1.ApiSpecification{
				Spec: roverv1.ApiSpecificationSpec{
					BasePath: "/ecc-pi/product-inventory-management-service/tmf-api/productInventory/v4",
				},
			}
			name := roverv1.MakeName(apiSpecification)
			Expect(name).To(Equal(expected))
		})

		It("should fail due to empty spec", func() {
			_, err := ParseSpecification(ctx, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				Equal("failed to parse specification: there is nothing in the spec, it's empty - so there is nothing to be done"),
			)
		})

		It("should successfully parse the v2 spec", func() {
			api, err := ParseSpecification(ctx, specV2)
			Expect(err).NotTo(HaveOccurred())

			Expect(api.Name).To(Equal("eni-foo-v1"))
			Expect(api.Spec.BasePath).To(Equal("/eni/foo/v1"))
			Expect(api.Spec.Version).To(Equal("1.0.0"))
			Expect(api.Spec.Category).To(Equal("test"))
			Expect(api.Spec.XVendor).To(BeTrue())
			Expect(api.Spec.Oauth2Scopes).To(ConsistOf("read", "write", "admin"))
		})

		It("should successfully parse the v3.0 spec", func() {
			api, err := ParseSpecification(ctx, specV3_0)
			Expect(err).NotTo(HaveOccurred())

			Expect(api.Name).To(Equal("eni-foo-v1"))
			Expect(api.Spec.BasePath).To(Equal("/eni/foo/v1"))
			Expect(api.Spec.Version).To(Equal("1.0.0"))
			Expect(api.Spec.Category).To(Equal("test"))
			Expect(api.Spec.XVendor).To(BeTrue())
			Expect(api.Spec.Oauth2Scopes).To(ConsistOf("read", "write", "admin"))
		})

		It("should not successfully parse the v3.0 spec without basepath", func() {
			_, err := ParseSpecification(ctx, specV3_0_without_basepath)
			Expect(err).To(HaveOccurred())
			Expect(problems.IsValidationError(err)).To(BeTrue())
		})

		It("should successfully parse the v3.1 spec", func() {
			api, err := ParseSpecification(ctx, specV3_1)
			Expect(err).NotTo(HaveOccurred())

			Expect(api.Spec.BasePath).To(Equal("/eni/foo/v1"))
			Expect(api.Spec.Version).To(Equal("1.0.0"))
			Expect(api.Spec.Category).To(Equal("test"))
			Expect(api.Spec.XVendor).To(BeTrue())
			Expect(api.Spec.Oauth2Scopes).To(ConsistOf("read", "write", "admin"))
		})

		It("should successfully parse the spec without scopes, category, vendor", func() {
			api, err := ParseSpecification(ctx, specNoExtraFields)
			Expect(err).NotTo(HaveOccurred())

			Expect(api.Spec.BasePath).To(Equal("/eni/foo/v1"))
			Expect(api.Spec.Version).To(Equal("1.0.0"))
			Expect(api.Spec.Category).To(Equal(""))
			Expect(api.Spec.XVendor).To(BeFalse())
			Expect(api.Spec.Oauth2Scopes).To(HaveLen(0))
		})

	})
	Context("When parsing OpenAPI v3 OAuth2 flows", func() {
		const header = `
openapi: "3.0.3"
info:
  version: "1.0.0"
  title: "Test API"
  x-api-category: "test"
  x-vendor: "true"
servers:
- url: "https://example.com/eni/foo/v1"
components:
  securitySchemes:
`

		parse := func(schemes string) *roverv1.ApiSpecification {
			api, err := ParseSpecification(ctx, header+schemes)
			Expect(err).NotTo(HaveOccurred())
			Expect(api.Spec.BasePath).To(Equal("/eni/foo/v1"))
			Expect(api.Spec.Category).To(Equal("test"))
			Expect(api.Spec.XVendor).To(BeTrue())
			return api
		}

		DescribeTable("should collect scopes from a single flow",
			func(flow, flowFields string) {
				api := parse(`
    oAuth2:
      type: oauth2
      flows:
        ` + flow + `:
` + flowFields + `
          scopes:
            write: write dummy
            read: read dummy
            admin: admin dummy
`)
				Expect(api.Spec.Oauth2Scopes).To(Equal([]string{"write", "read", "admin"}))
			},
			Entry("clientCredentials", "clientCredentials", "          tokenUrl: https://example.com/token"),
			Entry("password", "password", "          tokenUrl: https://example.com/token"),
			Entry("authorizationCode", "authorizationCode",
				"          authorizationUrl: https://example.com/auth\n          tokenUrl: https://example.com/token"),
			Entry("implicit", "implicit", "          authorizationUrl: https://example.com/auth"),
		)

		It("should collect scopes from an authorizationCode-only threeLegged scheme", func() {
			api := parse(`
    threeLegged:
      type: oauth2
      flows:
        authorizationCode:
          authorizationUrl: https://example.com/authorize
          tokenUrl: https://example.com/token
          scopes:
            qod-sessions-create: Create a QoD session
            qod-sessions-read: Read a QoD session
`)
			Expect(api.Spec.Oauth2Scopes).To(Equal([]string{"qod-sessions-create", "qod-sessions-read"}))
		})

		It("should collect and deduplicate scopes across mixed flows in deterministic order", func() {
			api := parse(`
    oAuth2:
      type: oauth2
      flows:
        implicit:
          authorizationUrl: https://example.com/auth
          scopes:
            implicit-scope: dummy
            shared: dummy
        authorizationCode:
          authorizationUrl: https://example.com/auth
          tokenUrl: https://example.com/token
          scopes:
            code-scope: dummy
            shared: dummy
        password:
          tokenUrl: https://example.com/token
          scopes:
            password-scope: dummy
        clientCredentials:
          tokenUrl: https://example.com/token
          scopes:
            cc-scope: dummy
            shared: dummy
`)
			Expect(api.Spec.Oauth2Scopes).To(Equal([]string{
				"cc-scope", "shared", "password-scope", "code-scope", "implicit-scope",
			}))
		})

		It("should collect and deduplicate scopes across multiple schemes and ignore non-oauth2 schemes", func() {
			api := parse(`
    apiKey:
      type: apiKey
      in: header
      name: X-API-Key
    zeta:
      type: oauth2
      flows:
        authorizationCode:
          authorizationUrl: https://example.com/auth
          tokenUrl: https://example.com/token
          scopes:
            write: dummy
            read: dummy
    alpha:
      type: oauth2
      flows:
        clientCredentials:
          tokenUrl: https://example.com/token
          scopes:
            admin: dummy
            write: dummy
`)
			// Scheme declaration order wins over flow order and alphabetical order.
			Expect(api.Spec.Oauth2Scopes).To(Equal([]string{"write", "read", "admin"}))
		})

		It("should handle oauth2 schemes with empty scopes or without flows", func() {
			api := parse(`
    noFlows:
      type: oauth2
    emptyFlows:
      type: oauth2
      flows: {}
    emptyScopes:
      type: oauth2
      flows:
        password:
          tokenUrl: https://example.com/token
          scopes: {}
        implicit:
          authorizationUrl: https://example.com/auth
`)
			Expect(api.Spec.Oauth2Scopes).NotTo(BeNil())
			Expect(api.Spec.Oauth2Scopes).To(BeEmpty())
		})
	})

	Context("server url path extraction", func() {
		specWithURL := func(u string) string {
			return "openapi: 3.0.3\ninfo:\n  title: t\n  version: \"1\"\nservers:\n  - url: '" + u + "'\n  - url: https://other/ignored\npaths: {}\n"
		}

		DescribeTable("should use the path of the first server",
			func(u, expected string) {
				api, err := ParseSpecification(ctx, specWithURL(u))
				Expect(err).NotTo(HaveOccurred())
				Expect(api.Spec.BasePath).To(Equal(expected))
			},
			Entry("templated authority without declarations", "https://{host}:{port}/eni/api/v1?x=1#f", "/eni/api/v1"),
			Entry("protocol-relative", "//{host}/eni/api/v1", "/eni/api/v1"),
			Entry("root-relative", "/eni/api/v1", "/eni/api/v1"),
			Entry("percent-encodings unchanged", "https://h/eni/my%20api%2Fx%7bv%7D", "/eni/my%20api%2Fx%7bv%7D"),
			Entry("reserved path chars", "https://h/a:b@c!$&()*+,;=-._~", "/a:b@c!$&()*+,;=-._~"),
			Entry("repeated slashes and dot segments", "https://h//eni/./api/../V1/", "//eni/./api/../V1/"),
			Entry("scheme without authority", "https:/eni/api/v1?x=1", "/eni/api/v1"),
			Entry("colon after first slash is path", "/eni/a://b", "/eni/a://b"),
		)

		It("should keep percent-encodings in the BasePath and name", func() {
			api, err := ParseSpecification(ctx, specWithURL("https://{host}/eni/My%20Api"))
			Expect(err).NotTo(HaveOccurred())
			Expect(api.Spec.BasePath).To(Equal("/eni/My%20Api"))
			Expect(roverv1.MakeName(api)).To(Equal("eni-my%20api"))
		})

		DescribeTable("should reject invalid server url paths",
			func(u string) {
				_, err := ParseSpecification(ctx, specWithURL(u))
				Expect(err).To(HaveOccurred())
				Expect(problems.IsValidationError(err)).To(BeTrue())
			},
			Entry("variable in path", "https://{host}/eni/{version}"),
			Entry("missing path", "https://{host}:{port}"),
			Entry("whole url variable", "{endpoint}"),
			Entry("whitespace", "https://h/eni/my api"),
			Entry("malformed escape", "https://h/eni/%zz"),
		)
	})

	DescribeTable("getPathFromURL",
		func(rawURL, expected string) {
			Expect(getPathFromURL(rawURL)).To(Equal(expected))
		},
		Entry("templated authority", "https://{host}:{port}/eni/api/v1?x=1#f", "/eni/api/v1"),
		Entry("percent-encodings unchanged", "/a%20b%2f%7B", "/a%20b%2f%7B"),
		Entry("reserved path chars", "/a:b@c!$&'()*+,;=-._~", "/a:b@c!$&'()*+,;=-._~"),
		Entry("repeated slashes and dot segments", "//h//a/./b/../C/", "//a/./b/../C/"),
		Entry("scheme without authority", "https:/eni/api/v1?x=1", "/eni/api/v1"),
		Entry("colon after first slash is path", "/eni/a://b", "/eni/a://b"),
		Entry("templated scheme", "{scheme}://{host}/eni/api", "/eni/api"),
	)

	DescribeTable("getPathFromURL errors",
		func(rawURL, msg string) {
			_, err := getPathFromURL(rawURL)
			Expect(err).To(MatchError(ContainSubstring(msg)))
		},
		Entry("variable in path", "https://h/eni/{version}", "must not contain variables"),
		Entry("space", "/eni/my api", "invalid character"),
		Entry("tab", "/eni/\tapi", "invalid character"),
		Entry("control char", "/eni/\x01api", "invalid character"),
		Entry("non-ascii", "/eni/äpi", "invalid character"),
		Entry("backslash", "/eni\\api", "invalid character"),
		Entry("quote", "/eni/\"api", "invalid character"),
		Entry("bad hex escape", "/eni/%zz", "malformed percent-encoding"),
		Entry("truncated escape", "/eni/%2", "malformed percent-encoding"),
	)
})
