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
})
