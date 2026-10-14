// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package security_test

import (
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
)

var _ = DescribeTable("DetermineClientType",
	func(scope string, expected security.ClientType) {
		actual, err := security.DetermineClientType([]string{scope}, "tardis")
		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(Equal(expected))
	},
	Entry("group", "tardis:group:read", security.ClientTypeGroup),
	Entry("legacy hub", "tardis:hub:read", security.ClientTypeGroup),
	Entry("team", "tardis:team:all", security.ClientTypeTeam),
	Entry("user alias", "tardis:user:read", security.ClientTypeTeam),
	Entry("admin", "tardis:admin:obfuscated", security.ClientTypeAdmin),
)

var _ = DescribeTable("rejects invalid client scopes",
	func(scope string) {
		_, err := security.DetermineClientType([]string{scope}, "tardis")
		Expect(err).To(HaveOccurred())
	},
	Entry("wrong prefix", "other:group:read"),
	Entry("invalid client type", "tardis:organization:read"),
)

var _ = Describe("BusinessContext scope resolution", func() {
	const defaultScope = "tardis:team:read"

	newToken := func(claims jwt.MapClaims) string {
		token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
		signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
		Expect(err).NotTo(HaveOccurred())
		return signed
	}

	tokenWithScope := func(scope string) string {
		return newToken(jwt.MapClaims{"env": "test", "clientId": "group--team--user", "scope": scope})
	}

	tokenWithoutScope := func() string {
		return newToken(jwt.MapClaims{"env": "test", "clientId": "group--team--user"})
	}

	// call runs the middleware and returns the status code, response body and resolved business context.
	call := func(token string, opts []security.Option[*security.BusinessContextOpts]) (int, string, *security.BusinessContext) {
		var bCtx *security.BusinessContext
		app := fiber.New()
		app.Use(mock.NewJWTMock())
		app.Use(security.NewBusinessCtxMiddlewareWithOpts(opts...))
		app.Get("/", func(c *fiber.Ctx) error {
			var ok bool
			bCtx, ok = security.FromContext(c.UserContext())
			Expect(ok).To(BeTrue())
			return c.SendStatus(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := app.Test(req, -1)
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			Expect(res.Body.Close()).To(Succeed())
		}()
		body, err := io.ReadAll(res.Body)
		Expect(err).NotTo(HaveOccurred())
		return res.StatusCode, string(body), bCtx
	}

	opts := func(prefix, defaultScope string) []security.Option[*security.BusinessContextOpts] {
		return []security.Option[*security.BusinessContextOpts]{
			security.WithScopePrefix(prefix),
			security.WithDefaultScope(defaultScope),
		}
	}
	withDefault := opts("tardis", defaultScope)
	withoutDefault := opts("tardis", "")

	DescribeTable("with a configured default scope",
		func(token func() string, expectedClient security.ClientType, expectedAccess security.AccessType) {
			status, _, bCtx := call(token(), withDefault)
			Expect(status).To(Equal(http.StatusOK))
			Expect(bCtx).NotTo(BeNil())
			Expect(bCtx.ClientType).To(Equal(expectedClient))
			Expect(bCtx.AccessType).To(Equal(expectedAccess))
		},
		Entry("uses the default when the scope claim contains only unknown scopes",
			func() string { return tokenWithScope("openid profile email") },
			security.ClientTypeTeam, security.AccessTypeReadOnly),
		Entry("uses the default when the scope claim contains only scopes with a foreign prefix",
			func() string { return tokenWithScope("other:admin:all") },
			security.ClientTypeTeam, security.AccessTypeReadOnly),
		Entry("uses prefixed scopes and ignores the default when other scopes are mixed in",
			func() string { return tokenWithScope("openid tardis:admin:all profile") },
			security.ClientTypeAdmin, security.AccessTypeReadWrite),
		Entry("ignores scopes whose first segment only starts with the prefix",
			func() string { return tokenWithScope("tardis-other:admin:all") },
			security.ClientTypeTeam, security.AccessTypeReadOnly),
		Entry("resolves the first matching client type and access type in claim order",
			func() string { return tokenWithScope("tardis:group:obfuscated tardis:admin:all") },
			security.ClientTypeGroup, security.AccessTypeObfuscated),
		Entry("resolves client type and access type independently across prefixed scopes",
			func() string { return tokenWithScope("tardis:admin:write tardis:organization:read") },
			security.ClientTypeAdmin, security.AccessTypeReadOnly),
		Entry("uses prefixed scopes when only known scopes are present",
			func() string { return tokenWithScope("tardis:group:obfuscated") },
			security.ClientTypeGroup, security.AccessTypeObfuscated),
		Entry("recognizes the legacy hub alias as group",
			func() string { return tokenWithScope("openid tardis:hub:all") },
			security.ClientTypeGroup, security.AccessTypeReadWrite),
		Entry("uses the default when the scope claim is absent",
			tokenWithoutScope,
			security.ClientTypeTeam, security.AccessTypeReadOnly),
		Entry("uses the default when the scope claim is empty",
			func() string { return tokenWithScope("") },
			security.ClientTypeTeam, security.AccessTypeReadOnly),
	)

	DescribeTable("defaults the scope prefix to tardis:",
		func(options []security.Option[*security.BusinessContextOpts], scope string, expectedClient security.ClientType, expectedAccess security.AccessType) {
			status, _, bCtx := call(tokenWithScope(scope), options)
			Expect(status).To(Equal(http.StatusOK))
			Expect(bCtx).NotTo(BeNil())
			Expect(bCtx.ClientType).To(Equal(expectedClient))
			Expect(bCtx.AccessType).To(Equal(expectedAccess))
		},
		Entry("when the prefix option is omitted, using tardis-prefixed scopes",
			[]security.Option[*security.BusinessContextOpts]{security.WithDefaultScope(defaultScope)},
			"openid tardis:admin:all", security.ClientTypeAdmin, security.AccessTypeReadWrite),
		Entry("when the prefix option is omitted, ignoring scopes of a foreign prefix",
			[]security.Option[*security.BusinessContextOpts]{security.WithDefaultScope(defaultScope)},
			"openid other:admin:all", security.ClientTypeTeam, security.AccessTypeReadOnly),
		Entry("when the prefix is explicitly empty, using tardis-prefixed scopes",
			opts("", defaultScope),
			"openid tardis:admin:all", security.ClientTypeAdmin, security.AccessTypeReadWrite),
		Entry("when the prefix is explicitly empty, ignoring scopes of a foreign prefix",
			opts("", defaultScope),
			"openid other:admin:all", security.ClientTypeTeam, security.AccessTypeReadOnly),
	)

	DescribeTable("with a custom scope prefix",
		func(scope string, expectedClient security.ClientType, expectedAccess security.AccessType) {
			status, _, bCtx := call(tokenWithScope(scope), opts("cp:", "cp:team:read"))
			Expect(status).To(Equal(http.StatusOK))
			Expect(bCtx).NotTo(BeNil())
			Expect(bCtx.ClientType).To(Equal(expectedClient))
			Expect(bCtx.AccessType).To(Equal(expectedAccess))
		},
		Entry("uses scopes with the custom prefix",
			"openid cp:admin:all", security.ClientTypeAdmin, security.AccessTypeReadWrite),
		Entry("ignores tardis-prefixed scopes",
			"openid tardis:admin:all", security.ClientTypeTeam, security.AccessTypeReadOnly),
	)

	DescribeTable("rejects requests",
		func(token func() string, options []security.Option[*security.BusinessContextOpts], expectedDetail string) {
			status, body, bCtx := call(token(), options)
			Expect(status).To(Equal(http.StatusForbidden))
			Expect(body).To(ContainSubstring(expectedDetail))
			Expect(bCtx).To(BeNil())
		},
		Entry("with only unknown scopes and no default",
			func() string { return tokenWithScope("openid profile") }, withoutDefault, "No valid scope found"),
		Entry("with an absent scope claim and no default",
			tokenWithoutScope, withoutDefault, "Missing field 'scope'"),
		Entry("with an empty scope claim and no default",
			func() string { return tokenWithScope("") }, withoutDefault, "Missing field 'scope'"),
		Entry("with a prefixed scope of an invalid client type despite a default",
			func() string { return tokenWithScope("openid tardis:organization:all") }, withDefault, "No valid scope found"),
		Entry("with a prefixed scope of an invalid access type despite a default",
			func() string { return tokenWithScope("openid tardis:admin:write") }, withDefault, "No valid scope found"),
		Entry("with an explicitly empty prefix, only unprefixed scopes and an unprefixed default",
			func() string { return tokenWithScope("openid admin:all") }, opts("", "team:read"), "No valid scope found"),
		Entry("with an omitted prefix, only unprefixed scopes and an unprefixed default",
			func() string { return tokenWithScope("openid admin:all") },
			[]security.Option[*security.BusinessContextOpts]{security.WithDefaultScope("team:read")}, "No valid scope found"),
		Entry("with only unknown scopes and an invalid default",
			func() string { return tokenWithScope("openid profile") }, opts("tardis", "tardis:organization:read"), "No valid scope found"),
		Entry("with an absent scope claim and a default of a foreign prefix",
			tokenWithoutScope, opts("tardis", "other:team:read"), "No valid scope found"),
	)
})
