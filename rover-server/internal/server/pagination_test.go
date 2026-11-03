// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/rover-server/internal/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPagination(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pagination links")
}

var _ = Describe("Pagination links", func() {
	It("encodes cursors and retains page size and other query parameters", func() {
		query := url.Values{"limit": {"2"}, "cursor": {"previous"}, "shallowInfo": {"true"}}
		link := buildPageURL("https://api.example", "/rovers/info", query, "team/a&b c")
		u, err := url.Parse(link)
		Expect(err).NotTo(HaveOccurred())
		Expect(u.Query().Get("cursor")).To(Equal("team/a&b c"))
		Expect(u.Query().Get("limit")).To(Equal("2"))
		Expect(u.Query().Get("shallowInfo")).To(Equal("true"))
		Expect(buildPageURL("https://api.example", "/rovers", url.Values{}, "")).To(Equal("https://api.example/rovers?limit=20"))
	})

	DescribeTable("builds rover links from the authenticated request path",
		func(lmsBasePath, claimPrefix, path string) {
			claims := jwt.MapClaims{"operation": "GET"}
			wantPath := path
			if claimPrefix != "" {
				claims["requestPath"] = claimPrefix + path
				wantPath = claimPrefix + path
			}
			links := getLinks(jwtApp(lmsBasePath), "http://rover.internal"+path+"?limit=2&cursor=old&shallowInfo=true", signToken(claims))

			for link, cursor := range map[string]string{links.Self: "self/c", links.Next: "next&c"} {
				u, err := url.Parse(link)
				Expect(err).NotTo(HaveOccurred())
				Expect(u.Scheme + "://" + u.Host + u.Path).To(Equal("http://rover.internal" + wantPath))
				Expect(u.Query().Get("cursor")).To(Equal(cursor))
				Expect(u.Query().Get("limit")).To(Equal("2"))
				Expect(u.Query().Get("shallowInfo")).To(Equal("true"))
			}
		},
		Entry("rovers with LMS /rover/api", "/rover/api", "/rover/api", "/rovers"),
		Entry("info with LMS /rover/api", "/rover/api", "/rover/api", "/rovers/info"),
		Entry("rovers with LMS /rover/v3", "/rover/v3", "/rover/v3", "/rovers"),
		Entry("info with LMS /rover/v3", "/rover/v3", "/rover/v3", "/rovers/info"),
		Entry("rovers without LMS under /rover/api", "", "/rover/api", "/rovers"),
		Entry("info without LMS under /rover/api", "", "/rover/api", "/rovers/info"),
		Entry("rovers without LMS under /rover/v3", "", "/rover/v3", "/rovers"),
		Entry("info without LMS under /rover/v3", "", "/rover/v3", "/rovers/info"),
		Entry("rovers without LMS or claim", "", "", "/rovers"),
		Entry("info without LMS or claim", "", "", "/rovers/info"),
	)

	It("does not trust unverified requestPath claims", func() {
		app := fiber.New()
		s := &Server{Rovers: pagingRovers{}}
		// A parsed but unverified token, as a mock or internal auth might leave it.
		app.Get("/rovers", func(c *fiber.Ctx) error {
			c.Locals("user", jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"requestPath": "/evil/rovers"}))
			return c.Next()
		}, s.GetAllRovers)
		links := getLinks(app, "http://rover.internal/rovers", "")
		Expect(links.Next).To(HavePrefix("http://rover.internal/rovers?"))
	})

	It("omits links for explicitly named info lookups", func() {
		app := jwtApp("/rover/api")
		token := signToken(jwt.MapClaims{"operation": "GET", "requestPath": "/rover/api/rovers/info"})
		links := getLinks(app, "http://rover.internal/rovers/info?names=a", token)
		Expect(links.Self).To(BeEmpty())
		Expect(links.Next).To(BeEmpty())
	})

	It("builds resources links from the authenticated path", func() {
		app := jwtApp("")
		token := signToken(jwt.MapClaims{"operation": "GET", "requestPath": "/rover/api/resources"})
		links := getLinks(app, "http://rover.internal/resources?group=g&team=t&limit=5", token)
		u, err := url.Parse(links.Next)
		Expect(err).NotTo(HaveOccurred())
		Expect(u.Path).To(Equal("/rover/api/resources"))
		Expect(u.Query().Get("cursor")).To(Equal("next&c"))
		Expect(u.Query().Get("limit")).To(Equal("5"))
		Expect(u.Query().Get("team")).To(Equal("t"))
	})

	It("builds legacy cursor links from the authenticated path", func() {
		app := jwtApp("/rover/api")
		token := signToken(jwt.MapClaims{"operation": "GET", "requestPath": "/rover/api/apiroadmaps"})
		links := getLinks(app, "http://rover.internal/apiroadmaps", token)
		Expect(links.Next).To(Equal("http://rover.internal/rover/api/apiroadmaps?cursor=next&c"))
	})

	It("falls back to the request path for legacy links without authentication", func() {
		links := getLinks(plainApp(), "http://rover.internal/apiroadmaps", "")
		Expect(links.Next).To(Equal("http://rover.internal/apiroadmaps?cursor=next&c"))
	})
})

var (
	testKey    *rsa.PrivateKey
	testIssuer *httptest.Server
)

var _ = BeforeSuite(func() {
	var err error
	testKey, err = rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())
	jwks := map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": "test", "alg": "RS256", "use": "sig",
		"n": base64.RawURLEncoding.EncodeToString(testKey.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(testKey.E)).Bytes()),
	}}}
	testIssuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		defer GinkgoRecover()
		w.Header().Set("Content-Type", "application/json")
		Expect(json.NewEncoder(w).Encode(jwks)).To(Succeed())
	}))
	DeferCleanup(testIssuer.Close)
})

func signToken(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test"
	signed, err := token.SignedString(testKey)
	Expect(err).NotTo(HaveOccurred())
	return signed
}

// jwtApp authenticates requests and optionally validates LMS claims.
func jwtApp(lmsBasePath string) *fiber.App {
	app := fiber.New()
	app.Use(security.NewJWTWithOpts(security.WithTrustedIssuers([]string{testIssuer.URL}), security.WithLmsCheck(lmsBasePath)))
	registerPaging(app)
	return app
}

func plainApp() *fiber.App {
	app := fiber.New()
	registerPaging(app)
	return app
}

func registerPaging(app *fiber.App) {
	s := &Server{Rovers: pagingRovers{}, Resources: pagingResources{}, Roadmaps: pagingRoadmaps{}}
	app.Get("/rovers", s.GetAllRovers)
	app.Get("/rovers/info", s.GetManyApplicationInfo)
	app.Get("/resources", s.GetAllResources)
	app.Get("/apiroadmaps", s.GetAllApiRoadmaps)
}

func getLinks(app *fiber.App, target, token string) api.Links {
	req, err := http.NewRequest(http.MethodGet, target, http.NoBody)
	Expect(err).NotTo(HaveOccurred())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := app.Test(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(res.StatusCode).To(Equal(http.StatusOK))
	var body struct {
		Links api.Links `json:"_links"`
	}
	Expect(json.NewDecoder(res.Body).Decode(&body)).To(Succeed())
	return body.Links
}

// pagingRovers returns fixed raw cursors that the handlers turn into links.
type pagingRovers struct{ RoverController }

func (pagingRovers) GetAll(context.Context, api.GetAllRoversParams) (*api.RoverListResponse, error) {
	return &api.RoverListResponse{UnderscoreLinks: api.Links{Self: "self/c", Next: "next&c"}}, nil
}

func (pagingRovers) GetApplicationsInfo(_ context.Context, params api.GetApplicationsInfoParams) (api.RoverInfoResponse, error) {
	if len(params.Names) > 0 {
		return api.RoverInfoResponse{}, nil
	}
	return api.RoverInfoResponse{UnderscoreLinks: api.Links{Self: "self/c", Next: "next&c"}}, nil
}

type pagingResources struct{ ResourcesController }

func (pagingResources) GetAll(context.Context, api.GetAllResourcesParams) (*api.ResourceListResponse, error) {
	return &api.ResourceListResponse{UnderscoreLinks: api.Links{Self: "self/c", Next: "next&c"}}, nil
}

type pagingRoadmaps struct{ ApiRoadmapController }

func (pagingRoadmaps) GetAll(context.Context, api.GetAllApiRoadmapsParams) (*api.ApiRoadmapListResponse, error) {
	return &api.ApiRoadmapListResponse{UnderscoreLinks: api.Links{Self: "self/c", Next: "next&c"}}, nil
}
