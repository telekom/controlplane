// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gofiber/fiber/v2"

	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

var _ = DescribeTable("decodeHeader",
	func(input, expected string) {
		Expect(decodeHeader(input)).To(Equal(expected))
	},
	Entry("plain ASCII", "John Doe", "John Doe"),
	Entry("encoded space", "John%20Doe", "John Doe"),
	Entry("encoded non-ASCII", "John%20D%C3%B6e", "John Döe"),
	Entry("fully encoded email", "user%40example.com", "user@example.com"),
	Entry("literal plus", "user+tag@example.com", "user+tag@example.com"),
	Entry("empty string", "", ""),
	Entry("invalid encoding falls back", "%zz-invalid", "%zz-invalid"),
	Entry("encoded comma-separated roles", "role%20one,role%20two", "role one,role two"),
)

var _ = Describe("Forwarded user HTTP ingress", func() {
	It("does not add an identity when optional headers are absent", func() {
		called := false
		app := fiber.New()
		app.Get("/", httpHandlerWithUserContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			_, ok := viewer.ForwardedUserFromContext(r.Context())
			Expect(ok).To(BeFalse())
			w.WriteHeader(http.StatusNoContent)
		})))
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(resp.Body.Close()).To(Succeed()) })
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		Expect(called).To(BeTrue())
	})

	DescribeTable("rejects malformed decoded email with an RFC9457 response", func(email string) {
		called := false
		app := fiber.New()
		app.Get("/", httpHandlerWithUserContext(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-User-Email", email)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(resp.Body.Close()).To(Succeed()) })
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))
		var problem map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&problem)).To(Succeed())
		Expect(problem["status"]).To(BeNumerically("==", http.StatusBadRequest))
		Expect(problem["detail"]).To(ContainSubstring("X-Forwarded-User-Email"))
		Expect(called).To(BeFalse())
	}, Entry("missing domain", "invalid"), Entry("display name", "Alice%20%3Calice%40example.com%3E"), Entry("whitespace", "%20alice%40example.com"), Entry("unquoted backslash", `ali\ce@example.com`))

	DescribeTable("preserves optional identity and valid original email", func(header, expected string) {
		var captured viewer.ForwardedUser
		app := fiber.New()
		app.Get("/", httpHandlerWithUserContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fu, ok := viewer.ForwardedUserFromContext(r.Context())
			Expect(ok).To(BeTrue())
			captured = fu
			w.WriteHeader(http.StatusNoContent)
		})))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-User-Name", "Alice%20D%C3%B6e")
		req.Header.Set("X-Forwarded-User-Email", header)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(resp.Body.Close()).To(Succeed()) })
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		Expect(captured.Name).To(Equal("Alice Döe"))
		Expect(captured.Email).To(Equal(expected))
	}, Entry("name only", "", ""), Entry("encoded Unicode", "%C3%BCser%40Example.COM", "üser@Example.COM"), Entry("literal tag", "Alice+Tag@Example.COM", "Alice+Tag@Example.COM"), Entry("quoted mailbox", "%22Alice%20Smith%22%40Example.COM", `"Alice Smith"@Example.COM`))
})

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
