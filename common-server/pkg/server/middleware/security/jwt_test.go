// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const testKid = "test-key"

// jwksIssuer serves a local JWKS for a generated RSA key.
func jwksIssuer(key *rsa.PrivateKey) *httptest.Server {
	jwks := map[string]any{"keys": []map[string]string{{
		"kty": "RSA",
		"kid": testKid,
		"alg": "RS256",
		"use": "sig",
		"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}}
	mux := http.NewServeMux()
	mux.HandleFunc("/protocol/openid-connect/certs", func(w http.ResponseWriter, _ *http.Request) {
		defer GinkgoRecover()
		w.Header().Set("Content-Type", "application/json")
		Expect(json.NewEncoder(w).Encode(jwks)).To(Succeed())
	})
	return httptest.NewServer(mux)
}

func signToken(key *rsa.PrivateKey, claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKid
	signed, err := token.SignedString(key)
	Expect(err).NotTo(HaveOccurred())
	return signed
}

var _ = Describe("JWT middleware authenticated request path", func() {
	var (
		key    *rsa.PrivateKey
		issuer *httptest.Server
	)

	BeforeEach(func() {
		var err error
		key, err = rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
		issuer = jwksIssuer(key)
		DeferCleanup(issuer.Close)
	})

	// call runs one request and returns the status and the published path
	// (empty body when no authenticated path was published).
	call := func(lmsBasePath, method, token string) (int, string) {
		app := fiber.New()
		app.Use(NewJWTWithOpts(WithTrustedIssuers([]string{issuer.URL}), WithLmsCheck(lmsBasePath)))
		app.All("/rovers", func(c *fiber.Ctx) error {
			path, ok := AuthenticatedRequestPath(c)
			if !ok {
				return c.SendString("")
			}
			return c.SendString(path)
		})
		req := httptest.NewRequest(method, "/rovers", http.NoBody)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		body, err := io.ReadAll(res.Body)
		Expect(err).NotTo(HaveOccurred())
		return res.StatusCode, string(body)
	}

	It("publishes the requestPath claim after LMS checks", func() {
		token := signToken(key, jwt.MapClaims{"operation": "get", "requestPath": "/rover/api/rovers"})
		status, path := call("/rover/api", http.MethodGet, token)
		Expect(status).To(Equal(http.StatusOK))
		Expect(path).To(Equal("/rover/api/rovers"))
	})

	DescribeTable("rejects invalid LMS claims without publishing a path",
		func(method string, claims jwt.MapClaims) {
			status, path := call("/rover/api", method, signToken(key, claims))
			Expect(status).To(Equal(http.StatusForbidden))
			Expect(path).NotTo(ContainSubstring("/rover/api/rovers"))
		},
		Entry("missing operation", http.MethodGet, jwt.MapClaims{"requestPath": "/rover/api/rovers"}),
		Entry("missing requestPath", http.MethodGet, jwt.MapClaims{"operation": "GET"}),
		Entry("wrong requestPath type", http.MethodGet, jwt.MapClaims{"operation": "GET", "requestPath": 42}),
		Entry("mismatched requestPath", http.MethodGet, jwt.MapClaims{"operation": "GET", "requestPath": "/other/rovers"}),
		Entry("mismatched method", http.MethodPost, jwt.MapClaims{"operation": "GET", "requestPath": "/rover/api/rovers"}),
	)

	It("publishes the authenticated requestPath claim when LMS is disabled", func() {
		token := signToken(key, jwt.MapClaims{"requestPath": "/gateway/rovers"})
		status, path := call("", http.MethodGet, token)
		Expect(status).To(Equal(http.StatusOK))
		Expect(path).To(Equal("/gateway/rovers"))
	})

	DescribeTable("publishes no path for unusable claims when LMS is disabled",
		func(claims jwt.MapClaims) {
			status, path := call("", http.MethodGet, signToken(key, claims))
			Expect(status).To(Equal(http.StatusOK))
			Expect(path).To(BeEmpty())
		},
		Entry("missing requestPath", jwt.MapClaims{"operation": "GET"}),
		Entry("empty requestPath", jwt.MapClaims{"requestPath": ""}),
		Entry("non-string requestPath", jwt.MapClaims{"requestPath": 42}),
	)

	It("publishes the claim verbatim when LMS matches case-insensitively", func() {
		token := signToken(key, jwt.MapClaims{"operation": "GET", "requestPath": "/rover/api/roverſ"})
		status, path := call("/rover/api", http.MethodGet, token)
		Expect(status).To(Equal(http.StatusOK))
		Expect(path).To(Equal("/rover/api/roverſ"))
	})

	DescribeTable("does not reach the handler when authentication fails",
		func(lmsBasePath string) {
			other, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).NotTo(HaveOccurred())
			token := signToken(other, jwt.MapClaims{"operation": "GET", "requestPath": "/rover/api/rovers"})
			status, path := call(lmsBasePath, http.MethodGet, token)
			Expect(status).To(Equal(http.StatusUnauthorized))
			Expect(path).NotTo(ContainSubstring("/rover/api/rovers"))
		},
		Entry("with LMS", "/rover/api"),
		Entry("without LMS", ""),
	)

	It("reports no authenticated path for requests without the middleware", func() {
		app := fiber.New()
		app.Get("/rovers", func(c *fiber.Ctx) error {
			c.Locals("requestPath", "/evil/rovers")
			c.Locals(UserContextKey, jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"requestPath": "/evil/rovers"}))
			_, ok := AuthenticatedRequestPath(c)
			Expect(ok).To(BeFalse())
			return nil
		})
		_, err := app.Test(httptest.NewRequest(http.MethodGet, "/rovers", http.NoBody))
		Expect(err).NotTo(HaveOccurred())
	})
})
