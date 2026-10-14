// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/mock"
	"k8s.io/utils/ptr"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/test/mocks/keycloakclient"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func isRetryable(err error) bool {
	var re ctrlerrors.RetryableError
	return errors.As(err, &re) && re.IsRetryable()
}

func isBlocked(err error) bool {
	var be ctrlerrors.BlockedError
	return errors.As(err, &be) && be.IsBlocked()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type closeErrBody struct {
	io.Reader
	err error
}

func (b *closeErrBody) Close() error { return b.err }

var _ = Describe("Realm attributes", func() {
	Describe("ValidateFrontendUrl", func() {
		DescribeTable("should accept valid base URLs",
			func(raw string) {
				Expect(keycloak.ValidateFrontendUrl(raw)).To(Succeed())
			},
			Entry("host only", "https://iam.example.com"),
			Entry("trailing slash", "https://iam.example.com/"),
			Entry("context path", "https://iam.example.com/auth"),
			Entry("context path with trailing slash", "https://iam.example.com/auth/"),
			Entry("http with port", "http://localhost:8080/auth"),
			Entry("path containing realms", "https://iam.example.com/realms/proxy"),
			Entry("upper-case scheme", "HTTPS://iam.example.com"),
		)

		It("should not retain a malformed credential-bearing URL in the error", func() {
			err := keycloak.ValidateFrontendUrl("https://user:s3cret@iam.example.com:bad/auth")
			Expect(err).To(MatchError(ContainSubstring("invalid URL")))
			Expect(err.Error()).ToNot(ContainSubstring("s3cret"))
			var urlErr *url.Error
			Expect(errors.As(err, &urlErr)).To(BeFalse())
		})

		DescribeTable("should reject invalid URLs",
			func(raw, expected string) {
				err := keycloak.ValidateFrontendUrl(raw)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expected))
			},
			Entry("empty", "", "scheme"),
			Entry("relative", "/auth", "scheme"),
			Entry("unsupported scheme", "ftp://iam.example.com", "scheme"),
			Entry("opaque", "https:iam.example.com", "host"),
			Entry("missing host", "https:///auth", "host"),
			Entry("credentials", "https://user:pw@iam.example.com", "credentials"),
			Entry("query", "https://iam.example.com/auth?a=b", "query"),
			Entry("empty query", "https://iam.example.com/auth?", "query"),
			Entry("fragment", "https://iam.example.com/auth#f", "fragment"),
			Entry("whitespace", " https://iam.example.com", "whitespace"),
			Entry("malformed", "https://iam.example.com:port", "invalid URL"),
		)
	})

	Describe("ValidateRealmAttributes", func() {
		It("should accept generic and empty attribute values", func() {
			Expect(keycloak.ValidateRealmAttributes(nil)).To(Succeed())
			Expect(keycloak.ValidateRealmAttributes(map[string]string{"displayName": "", "frontendUrl": ""})).To(Succeed())
		})

		It("should reject empty keys", func() {
			Expect(keycloak.ValidateRealmAttributes(map[string]string{"": "x"})).To(MatchError(ContainSubstring("must not be empty")))
		})

		It("should not validate the frontendUrl value", func() {
			Expect(keycloak.ValidateRealmAttributes(map[string]string{"frontendUrl": "iam.example.com"})).To(Succeed())
		})
	})

	Describe("IssuerUrlFromFrontendUrl", func() {
		DescribeTable("should derive the issuer like Keycloak",
			func(frontendUrl, realmName, expected string) {
				issuer, err := keycloak.IssuerUrlFromFrontendUrl(frontendUrl, realmName)
				Expect(err).ToNot(HaveOccurred())
				Expect(issuer).To(Equal(expected))
			},
			Entry("host only", "https://iam.example.com", "r", "https://iam.example.com/realms/r"),
			Entry("trailing slash", "https://iam.example.com/", "r", "https://iam.example.com/realms/r"),
			Entry("context path", "https://iam.example.com/auth", "r", "https://iam.example.com/auth/realms/r"),
			Entry("context path with trailing slash", "https://iam.example.com/auth/", "r", "https://iam.example.com/auth/realms/r"),
			Entry("non-default port", "http://localhost:8080/auth", "r", "http://localhost:8080/auth/realms/r"),
			Entry("default https port", "https://iam.example.com:443/auth", "r", "https://iam.example.com/auth/realms/r"),
			Entry("default http port", "http://iam.example.com:80", "r", "http://iam.example.com/realms/r"),
			Entry("escaped path", "https://iam.example.com/a%20b", "r", "https://iam.example.com/a%20b/realms/r"),
			Entry("escaped realm name", "https://iam.example.com", "a b", "https://iam.example.com/realms/a%20b"),
		)

		It("should reject an invalid frontend URL", func() {
			_, err := keycloak.IssuerUrlFromFrontendUrl("https://iam.example.com?x", "r")
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("ResolveIssuerUrl", func() {
	var (
		ctx        context.Context
		mockClient *keycloakclient.MockKeycloakClient
	)

	const publicIssuer = "https://iam.example.com/auth/realms/my-realm"

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = newMockClient()
	})

	newDiscoveryService := func(handler http.HandlerFunc) keycloak.KeycloakService {
		server := httptest.NewServer(handler)
		DeferCleanup(server.Close)
		return keycloak.NewKeycloakService(mockClient,
			keycloak.WithIssuerDiscovery(server.Client(), server.URL+"/auth/admin/realms/"))
	}

	It("should derive the issuer from the frontend URL without discovery", func() {
		svc := keycloak.NewKeycloakService(mockClient)
		issuer, err := svc.ResolveIssuerUrl(ctx, "my-realm", "https://iam.example.com/auth/")
		Expect(err).ToNot(HaveOccurred())
		Expect(issuer).To(Equal(publicIssuer))
	})

	It("should fall back to discovery for an invalid frontend URL", func() {
		svc := newDiscoveryService(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"issuer":%q}`, publicIssuer)
		})
		issuer, err := svc.ResolveIssuerUrl(ctx, "my-realm", "not a url")
		Expect(err).ToNot(HaveOccurred())
		Expect(issuer).To(Equal(publicIssuer))
	})

	It("should not trust a discovered issuer with credentials for an invalid frontend URL", func() {
		svc := newDiscoveryService(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"issuer":"https://user:pw@iam.example.com/realms/my-realm"}`)
		})
		_, err := svc.ResolveIssuerUrl(ctx, "my-realm", "https://user:pw@iam.example.com")
		Expect(err).To(HaveOccurred())
		Expect(isBlocked(err)).To(BeTrue())
		Expect(err.Error()).ToNot(ContainSubstring("pw"))
	})

	It("should not leak a malformed credential-bearing frontend URL", func() {
		var logs strings.Builder
		logCtx := logr.NewContext(ctx, funcr.New(func(prefix, args string) {
			logs.WriteString(prefix + args + "\n")
		}, funcr.Options{Verbosity: 1}))
		svc := newDiscoveryService(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"issuer":%q}`, publicIssuer)
		})

		issuer, err := svc.ResolveIssuerUrl(logCtx, "my-realm", "https://user:s3cret@iam.example.com:bad/auth")
		Expect(err).ToNot(HaveOccurred())
		Expect(issuer).To(Equal(publicIssuer))
		Expect(logs.String()).To(ContainSubstring("not a valid base URL"))
		Expect(logs.String()).ToNot(ContainSubstring("s3cret"))
	})

	It("should return a blocked error for an invalid frontend URL when discovery is not configured", func() {
		svc := keycloak.NewKeycloakService(mockClient)
		_, err := svc.ResolveIssuerUrl(ctx, "my-realm", "not a url")
		Expect(err).To(HaveOccurred())
		Expect(isBlocked(err)).To(BeTrue())
	})

	It("should return a blocked error when discovery is not configured", func() {
		svc := keycloak.NewKeycloakService(mockClient)
		_, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
		Expect(err).To(MatchError(ContainSubstring("OIDC discovery is not configured")))
		Expect(isBlocked(err)).To(BeTrue())
	})

	It("should return the advertised issuer, distinct from the management URL", func() {
		var requests []*http.Request
		svc := newDiscoveryService(func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r)
			fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":"x"}`, publicIssuer)
		})

		issuer, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
		Expect(err).ToNot(HaveOccurred())
		Expect(issuer).To(Equal(publicIssuer))

		Expect(requests).To(HaveLen(1))
		Expect(requests[0].URL.Path).To(Equal("/auth/realms/my-realm/.well-known/openid-configuration"))
		Expect(requests[0].Header.Get("Authorization")).To(BeEmpty())
	})

	It("should escape the realm name in the discovery URL", func() {
		var rawPath string
		svc := newDiscoveryService(func(w http.ResponseWriter, r *http.Request) {
			rawPath = r.URL.EscapedPath()
			fmt.Fprint(w, `{"issuer":"https://iam.example.com/realms/a%20b"}`)
		})

		issuer, err := svc.ResolveIssuerUrl(ctx, "a b", "")
		Expect(err).ToNot(HaveOccurred())
		Expect(issuer).To(Equal("https://iam.example.com/realms/a%20b"))
		Expect(rawPath).To(Equal("/auth/realms/a%20b/.well-known/openid-configuration"))
	})

	DescribeTable("should reject invalid discovery responses",
		func(status int, body, expected string, retryable bool) {
			svc := newDiscoveryService(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, body)
			})

			issuer, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
			Expect(err).To(MatchError(ContainSubstring(expected)))
			Expect(issuer).To(BeEmpty())
			Expect(isRetryable(err)).To(Equal(retryable))
			Expect(isBlocked(err)).To(Equal(!retryable))
		},
		Entry("server error", http.StatusServiceUnavailable, "", "HTTP 503", true),
		Entry("not found", http.StatusNotFound, "", "HTTP 404", true),
		Entry("malformed JSON", http.StatusOK, "{", "decoding", false),
		Entry("missing issuer", http.StatusOK, "{}", "invalid issuer URL", false),
		Entry("relative issuer", http.StatusOK, `{"issuer":"/realms/my-realm"}`, "invalid issuer URL", false),
		Entry("issuer with credentials", http.StatusOK, `{"issuer":"https://u:p@iam.example.com/realms/my-realm"}`, "credentials", false),
		Entry("issuer with query", http.StatusOK, `{"issuer":"https://iam.example.com/realms/my-realm?x=1"}`, "query", false),
		Entry("issuer with fragment", http.StatusOK, `{"issuer":"https://iam.example.com/realms/my-realm#x"}`, "fragment", false),
		Entry("malformed issuer with credentials", http.StatusOK, `{"issuer":"https://u:s3cret@iam.example.com:bad/realms/my-realm"}`, "invalid issuer URL", false),
		Entry("issuer of another realm", http.StatusOK, `{"issuer":"https://iam.example.com/realms/other"}`, "does not belong", false),
		Entry("issuer with realm suffix", http.StatusOK, `{"issuer":"https://iam.example.com/realms/xmy-realm"}`, "does not belong", false),
	)

	It("should reject an oversized discovery document", func() {
		svc := newDiscoveryService(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"issuer":%q,"padding":%q}`, publicIssuer, strings.Repeat("x", 1<<20))
		})

		_, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
		Expect(err).To(MatchError(ContainSubstring("decoding OIDC discovery document")))
		Expect(isBlocked(err)).To(BeTrue())
	})

	It("should not leak a malformed credential-bearing discovered issuer", func() {
		svc := newDiscoveryService(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"issuer":"https://u:s3cret@iam.example.com:bad/realms/my-realm"}`)
		})

		_, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).ToNot(ContainSubstring("s3cret"))
		var urlErr *url.Error
		Expect(errors.As(err, &urlErr)).To(BeFalse())
	})

	Describe("closing the discovery response", func() {
		newClosingService := func(body string, status int, closeErr error) keycloak.KeycloakService {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: status,
					Body:       &closeErrBody{Reader: strings.NewReader(body), err: closeErr},
					Header:     http.Header{},
				}, nil
			})}
			return keycloak.NewKeycloakService(mockClient,
				keycloak.WithIssuerDiscovery(client, "https://iam.example.com/auth/admin/realms/"))
		}

		It("should return a close error after a successful read", func() {
			svc := newClosingService(fmt.Sprintf(`{"issuer":%q}`, publicIssuer), http.StatusOK, errors.New("close failed"))
			_, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
			Expect(err).To(MatchError(ContainSubstring("closing OIDC discovery response: close failed")))
		})

		It("should keep the primary error and its classification on close failure", func() {
			svc := newClosingService("", http.StatusServiceUnavailable, errors.New("close failed"))
			_, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
			Expect(err).To(MatchError(ContainSubstring("HTTP 503")))
			Expect(err).To(MatchError(ContainSubstring("close failed")))
			Expect(isRetryable(err)).To(BeTrue())
		})

		It("should succeed when closing succeeds", func() {
			svc := newClosingService(fmt.Sprintf(`{"issuer":%q}`, publicIssuer), http.StatusOK, nil)
			issuer, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(issuer).To(Equal(publicIssuer))
		})
	})

	It("should block discovery without a request or leak for a malformed admin URL", func() {
		var requested bool
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			requested = true
			return nil, errors.New("unexpected request")
		})}
		svc := keycloak.NewKeycloakService(mockClient,
			keycloak.WithIssuerDiscovery(client, "https://user:s3cret@iam.example.com:bad/auth/admin/realms/"))

		_, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
		Expect(err).To(MatchError(ContainSubstring("determining OIDC discovery URL: invalid admin URL")))
		Expect(err.Error()).ToNot(ContainSubstring("s3cret"))
		Expect(isBlocked(err)).To(BeTrue())
		Expect(requested).To(BeFalse())
	})

	It("should accept a discovery document of exactly the size limit", func() {
		prefix := fmt.Sprintf(`{"issuer":%q,"padding":"`, publicIssuer)
		document := prefix + strings.Repeat("x", (1<<20)-len(prefix)-len(`"}`)) + `"}`
		Expect(document).To(HaveLen(1 << 20))
		svc := newDiscoveryService(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, document)
		})

		issuer, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
		Expect(err).ToNot(HaveOccurred())
		Expect(issuer).To(Equal(publicIssuer))
	})

	It("should return a retryable error when the server is unreachable", func() {
		server := httptest.NewServer(http.NotFoundHandler())
		serverUrl := server.URL
		server.Close()

		svc := keycloak.NewKeycloakService(mockClient, keycloak.WithIssuerDiscovery(http.DefaultClient, serverUrl+"/admin/realms/"))
		_, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
		Expect(err).To(MatchError(ContainSubstring("requesting OIDC discovery document")))
		Expect(isRetryable(err)).To(BeTrue())
	})

	It("should fail on untrusted TLS certificates", func() {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"issuer":%q}`, publicIssuer)
		}))
		DeferCleanup(server.Close)

		svc := keycloak.NewKeycloakService(mockClient,
			keycloak.WithIssuerDiscovery(&http.Client{}, server.URL+"/auth/admin/realms/"))
		_, err := svc.ResolveIssuerUrl(ctx, "my-realm", "")
		Expect(err).To(MatchError(ContainSubstring("certificate")))
		Expect(isRetryable(err)).To(BeTrue())
	})
})

var _ = Describe("CreateOrReplaceRealm attributes", func() {
	var (
		ctx        context.Context
		mockClient *keycloakclient.MockKeycloakClient
		svc        keycloak.KeycloakService
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = newMockClient()
		svc = keycloak.NewKeycloakService(mockClient)
	})

	existingRealm := func(attributes map[string]interface{}) {
		mockClient.EXPECT().GetRealmWithResponse(mock.Anything, "my-realm").
			Return(&api.GetRealmResponse{
				HTTPResponse: httpResp(200),
				JSON2XX: &api.RealmRepresentation{
					Realm:       ptr.To("my-realm"),
					Enabled:     ptr.To(true),
					DisplayName: ptr.To("kept"),
					Attributes:  &attributes,
				},
			}, nil)
	}

	// capturePut records the JSON body that would be sent to Keycloak.
	capturePut := func() *map[string]interface{} {
		var wire map[string]interface{}
		mockClient.EXPECT().PutRealmWithResponse(mock.Anything, "my-realm", mock.Anything).
			RunAndReturn(func(_ context.Context, _ string, body api.RealmRepresentation, _ ...api.RequestEditorFn) (*api.PutRealmResponse, error) {
				raw, err := json.Marshal(body)
				Expect(err).ToNot(HaveOccurred())
				Expect(json.Unmarshal(raw, &wire)).To(Succeed())
				return &api.PutRealmResponse{HTTPResponse: httpResp(204)}, nil
			})
		return &wire
	}

	It("should create a realm with attributes", func() {
		realm := newRealm("my-realm")
		realm.Spec.Attributes = map[string]string{"frontendUrl": "https://iam.example.com/auth", "b": "2"}
		mockClient.EXPECT().GetRealmWithResponse(mock.Anything, "my-realm").
			Return(&api.GetRealmResponse{HTTPResponse: httpResp(404)}, nil)
		mockClient.EXPECT().PostWithResponse(mock.Anything, mock.MatchedBy(func(body api.RealmRepresentation) bool {
			return body.Attributes != nil && (*body.Attributes)["b"] == "2"
		})).Return(&api.PostResponse{HTTPResponse: httpResp(201)}, nil)

		result, err := svc.CreateOrReplaceRealm(ctx, realm)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.ManagedAttributes).To(Equal([]string{"b", "frontendUrl"}))
		Expect(result.FrontendUrl).To(Equal("https://iam.example.com/auth"))
	})

	It("should send the full attribute set and omit removed managed keys on the wire", func() {
		realm := newRealm("my-realm")
		realm.Spec.Attributes = map[string]string{"a": "new"}
		realm.Status.ManagedAttributes = []string{"a", "removed"}
		existingRealm(map[string]interface{}{"a": "old", "removed": "x", "manual": "m"})
		wire := capturePut()

		result, err := svc.CreateOrReplaceRealm(ctx, realm)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.ManagedAttributes).To(Equal([]string{"a"}))

		// Keycloak removes attributes missing from a non-null attributes map.
		Expect((*wire)["attributes"]).To(Equal(map[string]interface{}{"a": "new", "manual": "m"}))
		Expect((*wire)["displayName"]).To(Equal("kept"))
	})

	It("should send an empty attributes object when the last managed key is removed", func() {
		realm := newRealm("my-realm")
		realm.Status.ManagedAttributes = []string{"a"}
		existingRealm(map[string]interface{}{"a": "1"})
		wire := capturePut()

		result, err := svc.CreateOrReplaceRealm(ctx, realm)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.ManagedAttributes).To(BeEmpty())
		Expect(*wire).To(HaveKeyWithValue("attributes", map[string]interface{}{}))
	})

	It("should not touch attributes of an upgraded realm without managed attributes", func() {
		realm := newRealm("my-realm")
		existingRealm(map[string]interface{}{"manual": "m", "frontendUrl": "https://manual.example.com"})

		result, err := svc.CreateOrReplaceRealm(ctx, realm)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.ManagedAttributes).To(BeEmpty())
		By("reporting the manually configured frontendUrl as effective")
		Expect(result.FrontendUrl).To(Equal("https://manual.example.com"))
	})

	It("should skip the update when attributes are already applied", func() {
		realm := newRealm("my-realm")
		realm.Spec.Attributes = map[string]string{"a": "1"}
		realm.Status.ManagedAttributes = []string{"a"}
		existingRealm(map[string]interface{}{"a": "1", "manual": "m"})

		result, err := svc.CreateOrReplaceRealm(ctx, realm)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.ManagedAttributes).To(Equal([]string{"a"}))
	})

	It("should report the desired frontendUrl after an update", func() {
		realm := newRealm("my-realm")
		realm.Spec.Attributes = map[string]string{"frontendUrl": "https://iam.example.com"}
		existingRealm(map[string]interface{}{"frontendUrl": "https://manual.example.com"})
		wire := capturePut()

		result, err := svc.CreateOrReplaceRealm(ctx, realm)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.FrontendUrl).To(Equal("https://iam.example.com"))
		Expect((*wire)["attributes"]).To(HaveKeyWithValue("frontendUrl", "https://iam.example.com"))
	})

	It("should return no result when the update fails", func() {
		realm := newRealm("my-realm")
		realm.Spec.Attributes = map[string]string{"a": "1"}
		existingRealm(map[string]interface{}{})
		mockClient.EXPECT().PutRealmWithResponse(mock.Anything, "my-realm", mock.Anything).
			Return(&api.PutRealmResponse{HTTPResponse: httpResp(500)}, nil)

		result, err := svc.CreateOrReplaceRealm(ctx, realm)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
	})
})
