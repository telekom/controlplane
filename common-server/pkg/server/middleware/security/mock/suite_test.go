// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mock_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
)

func TestMock(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mock Suite")
}

var _ = Describe("JWT Mock Middleware", func() {

	Context("Mock Token", func() {

		It("should return a valid mock token", func() {
			token := mock.NewMockAccessToken("test", "group", "team", nil)
			Expect(token).NotTo(BeEmpty())
			claims := jwt.MapClaims{}
			_, _, err := jwt.NewParser().ParseUnverified(token, &claims)
			Expect(err).ToNot(HaveOccurred())

			Expect(claims["env"]).To(Equal("test"))
			Expect(claims["clientId"]).To(Equal("group--team"))
		})
	})

	Context("Mock Middleware", func() {

		app := fiber.New()
		app.Use(mock.NewJWTMock())

		It("should return unauthorized if not auth header is provided", func() {
			req := httptest.NewRequest("GET", "/", nil)

			res, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.StatusCode).To(Equal(401))
		})

		It("should return unauthorized if invalid auth header is provided", func() {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", "Bearer")

			res, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.StatusCode).To(Equal(401))
		})

		It("should return unauthorized if invalid token is provided", func() {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", "Bearer invalid")

			res, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.StatusCode).To(Equal(401))
		})

		It("should return success if valid token is provided", func() {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", "Bearer "+mock.NewMockAccessToken("test", "group", "team", nil))

			res, err := app.Test(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).NotTo(BeNil())
			Expect(res.StatusCode).To(Equal(404))
		})

	})
})
