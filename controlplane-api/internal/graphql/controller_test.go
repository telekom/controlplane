// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gofiber/fiber/v2"

	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

var _ = DescribeTable("decodeHeader",
	func(input, expected string) {
		Expect(decodeHeader(input)).To(Equal(expected))
	},
	Entry("plain ASCII", "John Doe", "John Doe"),
	Entry("encoded space", "John%20Doe", "John Doe"),
	Entry("encoded non-ASCII", "John%20D%C3%B6e", "John Döe"),
	Entry("fully encoded email", "user%40example.com", "user@example.com"),
	Entry("empty string", "", ""),
	Entry("invalid encoding falls back", "%zz-invalid", "%zz-invalid"),
	Entry("encoded comma-separated roles", "role%20one,role%20two", "role one,role two"),
)

var _ = Describe("Controller routes", func() {
	newApp := func(playground bool) *fiber.App {
		app := fiber.New()
		opts := security.SecurityOpts{Mode: security.ModeMock, DisableGlobalGuard: true}
		guard := cserver.JWTFamily(opts)(app)
		NewController(nil, playground).RegisterRoutes(app, guard)
		return app
	}

	It("serves the enabled playground without authentication", func() {
		resp, err := newApp(true).Test(httptest.NewRequest(http.MethodGet, "/graphql", nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("does not expose a disabled playground", func() {
		resp, err := newApp(false).Test(httptest.NewRequest(http.MethodGet, "/graphql", nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})

	DescribeTable("requires authentication for GraphQL queries", func(method string) {
		body := strings.NewReader(`{"query":"{ __schema { queryType { name } } }"}`)
		resp, err := newApp(true).Test(httptest.NewRequest(method, "/graphql/query", body))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
	},
		Entry("over GET", http.MethodGet),
		Entry("over POST", http.MethodPost),
	)
})
