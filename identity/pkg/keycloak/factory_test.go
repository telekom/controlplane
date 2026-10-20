// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	k8srand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/telekom/controlplane/common-server/pkg/client/tlsclient"
	"github.com/telekom/controlplane/common/pkg/cabundle"
	"github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{}
	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
})

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestCA(cn string) *testCA {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	cert, err := x509.ParseCertificate(der)
	Expect(err).NotTo(HaveOccurred())
	return &testCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// startTLSServer serves h with a certificate for 127.0.0.1 issued by ca.
func startTLSServer(ca *testCA, h http.Handler) string {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	Expect(err).NotTo(HaveOccurred())

	srv := httptest.NewUnstartedServer(h)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		MinVersion:   tls.VersionTLS13,
	}
	srv.StartTLS()
	DeferCleanup(srv.Close)
	return srv.URL
}

func startServer(h http.Handler) string {
	srv := httptest.NewServer(h)
	DeferCleanup(srv.Close)
	return srv.URL
}

// tokenServer is a password grant token endpoint with configurable failures.
type tokenServer struct {
	passwordCalls  atomic.Int32
	refreshCalls   atomic.Int32
	passwordStatus atomic.Int32
	refreshStatus  atomic.Int32
	expiresIn      atomic.Int32
}

func (t *tokenServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var status int32
	switch r.Form.Get("grant_type") {
	case "password":
		t.passwordCalls.Add(1)
		status = t.passwordStatus.Load()
	case "refresh_token":
		t.refreshCalls.Add(1)
		status = t.refreshStatus.Load()
	}
	w.Header().Set("Content-Type", "application/json")
	if status != 0 {
		w.WriteHeader(int(status))
		_, _ = w.Write([]byte(`{"error":"failed"}`))
		return
	}
	expires := int(t.expiresIn.Load())
	if expires == 0 {
		expires = 3600
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  "token-" + r.Form.Get("grant_type"),
		"refresh_token": "refresh",
		"token_type":    "Bearer",
		"expires_in":    expires,
	})
}

// apiHandler records the Authorization header of API requests.
type apiHandler struct {
	mu   sync.Mutex
	auth []string
}

func (a *apiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.auth = append(a.auth, r.Header.Get("Authorization"))
	a.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (a *apiHandler) last() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.auth) == 0 {
		return ""
	}
	return a.auth[len(a.auth)-1]
}

type failingReader struct{ client.Reader }

func (failingReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("connection refused")
}

func doGet(c *tlsclient.Client) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.APIURL()+"/realms", http.NoBody)
	Expect(err).NotTo(HaveOccurred())
	resp, err := c.Doer().Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func isBlocked(err error) bool {
	var be interface{ IsBlocked() bool }
	return errors.As(err, &be) && be.IsBlocked()
}

var _ = Describe("CachedServiceFactory", func() {
	var (
		ctx       context.Context
		namespace string
		factory   *CachedServiceFactory
		tokens    *tokenServer
		api       *apiHandler
	)

	newRealm := func(name, uid string) *identityv1.Realm {
		return &identityv1.Realm{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "realm-ns", UID: k8stypes.UID(uid)}}
	}

	statusFor := func(apiURL, tokenURL string, conn *identityv1.RealmConnection) identityv1.RealmStatus {
		return identityv1.RealmStatus{
			AdminClientId: "admin-cli",
			AdminUserName: "admin",
			AdminPassword: "password",
			AdminUrl:      apiURL,
			AdminTokenUrl: tokenURL,
			Connection:    conn,
		}
	}

	idpConnection := func(ref *types.ConfigMapKeyRef) *identityv1.RealmConnection {
		conn := &identityv1.RealmConnection{IdentityProvider: types.ObjectRef{Name: "idp", Namespace: namespace, UID: "idp-uid"}}
		if ref != nil {
			conn.TLS = &identityv1.RealmConnectionTLS{Namespace: namespace, ConfigMapRef: *ref}
		}
		return conn
	}

	writeBundle := func(name, data string) {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
		cm.Data = map[string]string{"ca.pem": data}
		if err == nil {
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())
			return
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
	}

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "keycloak-" + k8srand.String(6)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
		tokens = &tokenServer{}
		api = &apiHandler{}

		var err error
		factory, err = NewServiceFactory(context.Background(), k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(factory.NeedLeaderElection()).To(BeFalse())
		startCtx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(done)
			Expect(factory.Start(startCtx)).To(Succeed())
		}()
		DeferCleanup(func() {
			cancel()
			Eventually(done).WithTimeout(5 * time.Second).WithPolling(10 * time.Millisecond).Should(BeClosed())
		})
	})

	Context("TLS", func() {
		var (
			tokenCA, apiCA   *testCA
			apiURL, tokenURL string
		)

		BeforeEach(func() {
			tokenCA = newTestCA("token-ca")
			apiCA = newTestCA("api-ca")
			tokenURL = startTLSServer(tokenCA, tokens) + "/realms/master/protocol/openid-connect/token"
			apiURL = startTLSServer(apiCA, api)
		})

		It("uses the union CA bundle for token and API requests", func() {
			union, err := cabundle.Canonicalize(tokenCA.pem, apiCA.pem)
			Expect(err).NotTo(HaveOccurred())
			writeBundle("bundle", string(union))
			status := statusFor(apiURL, tokenURL, idpConnection(&types.ConfigMapKeyRef{Name: "bundle", Key: "ca.pem"}))

			lease, err := factory.acquire(ctx, newRealm("realm", "realm-uid"), &status)
			Expect(err).NotTo(HaveOccurred())
			defer lease.Release()
			Expect(tokens.passwordCalls.Load()).To(BeEquivalentTo(1))

			Expect(doGet(lease.Client())).To(Succeed())
			Expect(api.last()).To(Equal("Bearer token-password"))

			svc, release, err := factory.ServiceFor(ctx, newRealm("realm", "realm-uid"), status)
			Expect(err).NotTo(HaveOccurred())
			Expect(svc).NotTo(BeNil())
			release()
		})

		It("fails the eager authentication if the token endpoint is not trusted", func() {
			writeBundle("bundle", string(apiCA.pem))
			status := statusFor(apiURL, tokenURL, idpConnection(&types.ConfigMapKeyRef{Name: "bundle", Key: "ca.pem"}))

			_, _, err := factory.ServiceFor(ctx, newRealm("realm", "realm-uid"), status)
			Expect(err).To(MatchError(ContainSubstring("certificate")))
			Expect(isBlocked(err)).To(BeFalse())
		})

		It("enforces API verification even with the legacy bypass environment variable", func() {
			GinkgoT().Setenv("SKIP_TLS_VERIFY", "true")
			writeBundle("bundle", string(tokenCA.pem))
			status := statusFor(apiURL, tokenURL, idpConnection(&types.ConfigMapKeyRef{Name: "bundle", Key: "ca.pem"}))
			lease, err := factory.acquire(ctx, newRealm("realm", "realm-uid"), &status)
			Expect(err).NotTo(HaveOccurred())
			defer lease.Release()
			Expect(doGet(lease.Client())).To(MatchError(ContainSubstring("certificate")))
			Expect(api.last()).To(BeEmpty())
		})

		It("discovers the issuer with custom trust without sending admin credentials", func() {
			var discoveryAuth string
			var discoveryCalls int
			discoveryURL := startTLSServer(apiCA, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				discoveryAuth = r.Header.Get("Authorization")
				discoveryCalls++
				Expect(r.URL.Path).To(Equal("/auth/realms/test/.well-known/openid-configuration"))
				_, err := fmt.Fprint(w, `{"issuer":"https://public.example.com/auth/realms/test"}`)
				Expect(err).NotTo(HaveOccurred())
			}))
			union, err := cabundle.Canonicalize(tokenCA.pem, apiCA.pem)
			Expect(err).NotTo(HaveOccurred())
			writeBundle("bundle", string(union))
			status := statusFor(discoveryURL+"/auth/admin/realms/", tokenURL, idpConnection(&types.ConfigMapKeyRef{Name: "bundle", Key: "ca.pem"}))
			svc, release, err := factory.ServiceFor(ctx, newRealm("realm", "realm-uid"), status)
			Expect(err).NotTo(HaveOccurred())
			defer release()
			issuer, err := svc.ResolveIssuerUrl(ctx, "test", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(issuer).To(Equal("https://public.example.com/auth/realms/test"))
			Expect(discoveryCalls).To(Equal(1))
			Expect(discoveryAuth).To(BeEmpty())

			By("rejecting an untrusted discovery endpoint even with the legacy environment variable")
			GinkgoT().Setenv("SKIP_TLS_VERIFY", "true")
			writeBundle("bundle", string(tokenCA.pem))
			untrusted, releaseUntrusted, err := factory.ServiceFor(ctx, newRealm("realm", "realm-uid"), status)
			Expect(err).NotTo(HaveOccurred())
			defer releaseUntrusted()
			_, err = untrusted.ResolveIssuerUrl(ctx, "test", "")
			Expect(err).To(MatchError(ContainSubstring("certificate")))
			Expect(discoveryCalls).To(Equal(1))
		})

		It("uses system roots only for legacy statuses", func() {
			status := statusFor(apiURL, tokenURL, nil)
			_, _, err := factory.ServiceFor(ctx, newRealm("realm", "realm-uid"), status)
			Expect(err).To(MatchError(ContainSubstring("certificate")))
		})

		It("re-validates the CA bundle before serving a cached client", func() {
			union, err := cabundle.Canonicalize(tokenCA.pem, apiCA.pem)
			Expect(err).NotTo(HaveOccurred())
			writeBundle("bundle", string(union))
			status := statusFor(apiURL, tokenURL, idpConnection(&types.ConfigMapKeyRef{Name: "bundle", Key: "ca.pem"}))
			realm := newRealm("realm", "realm-uid")

			_, release, err := factory.ServiceFor(ctx, realm, status)
			Expect(err).NotTo(HaveOccurred())
			release()

			writeBundle("bundle", "junk")
			_, _, err = factory.ServiceFor(ctx, realm, status)
			Expect(isBlocked(err)).To(BeTrue(), "invalid source must block: %v", err)
			Expect(errors.Is(err, cabundle.ErrInvalidBundle)).To(BeTrue())

			Expect(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "bundle", Namespace: namespace}})).To(Succeed())
			_, _, err = factory.ServiceFor(ctx, realm, status)
			Expect(isBlocked(err)).To(BeTrue(), "missing source must block: %v", err)
			Expect(tokens.passwordCalls.Load()).To(BeEquivalentTo(1))
		})
	})

	Context("owner isolation", func() {
		var apiURL, tokenURL string

		BeforeEach(func() {
			tokenURL = startServer(tokens) + "/token"
			apiURL = startServer(api)
		})

		It("scopes legacy clients to the consuming Realm", func() {
			status := statusFor(apiURL, tokenURL, nil)
			a1, err := factory.acquire(ctx, newRealm("a", "uid-a"), &status)
			Expect(err).NotTo(HaveOccurred())
			defer a1.Release()
			a2, err := factory.acquire(ctx, newRealm("a", "uid-a"), &status)
			Expect(err).NotTo(HaveOccurred())
			defer a2.Release()
			b, err := factory.acquire(ctx, newRealm("b", "uid-b"), &status)
			Expect(err).NotTo(HaveOccurred())
			defer b.Release()

			Expect(a1.Client()).To(BeIdenticalTo(a2.Client()))
			Expect(b.Client()).NotTo(BeIdenticalTo(a1.Client()))
			Expect(tokens.passwordCalls.Load()).To(BeEquivalentTo(2))
		})

		It("shares clients of the same IdentityProvider across Realms but not across snapshots", func() {
			status := statusFor(apiURL, tokenURL, idpConnection(nil))
			a, err := factory.acquire(ctx, newRealm("a", "uid-a"), &status)
			Expect(err).NotTo(HaveOccurred())
			defer a.Release()
			b, err := factory.acquire(ctx, newRealm("b", "uid-b"), &status)
			Expect(err).NotTo(HaveOccurred())
			defer b.Release()
			Expect(b.Client()).To(BeIdenticalTo(a.Client()))

			rotated := status
			rotated.AdminPassword = "rotated"
			c, err := factory.acquire(ctx, newRealm("c", "uid-c"), &rotated)
			Expect(err).NotTo(HaveOccurred())
			defer c.Release()
			Expect(c.Client()).NotTo(BeIdenticalTo(a.Client()))

			other := statusFor(apiURL, tokenURL, idpConnection(nil))
			other.Connection.IdentityProvider.UID = "other-idp-uid"
			d, err := factory.acquire(ctx, newRealm("a", "uid-a"), &other)
			Expect(err).NotTo(HaveOccurred())
			defer d.Release()
			Expect(d.Client()).NotTo(BeIdenticalTo(a.Client()))
		})
	})

	Context("connection metadata", func() {
		DescribeTable("blocks incomplete metadata without network calls",
			func(mutate func(*identityv1.RealmConnection)) {
				conn := idpConnection(&types.ConfigMapKeyRef{Name: "bundle", Key: "ca.pem"})
				mutate(conn)
				status := statusFor("http://127.0.0.1:1", "http://127.0.0.1:1/token", conn)
				_, _, err := factory.ServiceFor(ctx, newRealm("realm", "uid"), status)
				Expect(err).To(MatchError(ContainSubstring("incomplete connection metadata")))
				Expect(isBlocked(err)).To(BeTrue())
			},
			Entry("missing uid", func(c *identityv1.RealmConnection) { c.IdentityProvider.UID = "" }),
			Entry("missing name", func(c *identityv1.RealmConnection) { c.IdentityProvider.Name = "" }),
			Entry("missing namespace", func(c *identityv1.RealmConnection) { c.IdentityProvider.Namespace = "" }),
			Entry("missing tls namespace", func(c *identityv1.RealmConnection) { c.TLS.Namespace = "" }),
			Entry("missing configmap name", func(c *identityv1.RealmConnection) { c.TLS.ConfigMapRef.Name = "" }),
			Entry("missing configmap key", func(c *identityv1.RealmConnection) { c.TLS.ConfigMapRef.Key = "" }),
		)

		It("rejects a nil Realm", func() {
			_, _, err := factory.ServiceFor(ctx, nil, statusFor("http://127.0.0.1:1", "http://127.0.0.1:1/token", nil))
			Expect(err).To(MatchError(ContainSubstring("realm is nil")))
		})

		It("fails if a CA reference is configured but no reader is available", func() {
			f, err := NewServiceFactory(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(f.clients.Close)
			status := statusFor("http://127.0.0.1:1", "http://127.0.0.1:1/token", idpConnection(&types.ConfigMapKeyRef{Name: "b", Key: "k"}))
			_, _, err = f.ServiceFor(ctx, newRealm("realm", "uid"), status)
			Expect(err).To(MatchError(ContainSubstring("no resolver is configured")))
		})

		It("retries transient read errors", func() {
			f, err := NewServiceFactory(context.Background(), failingReader{})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(f.clients.Close)
			status := statusFor("http://127.0.0.1:1", "http://127.0.0.1:1/token", idpConnection(&types.ConfigMapKeyRef{Name: "b", Key: "k"}))
			_, _, err = f.ServiceFor(ctx, newRealm("realm", "uid"), status)
			Expect(err).To(MatchError(ContainSubstring("connection refused")))
			Expect(isBlocked(err)).To(BeFalse())
		})
	})

	Context("authentication", func() {
		var apiURL, tokenURL string

		BeforeEach(func() {
			tokenURL = startServer(tokens) + "/token"
			apiURL = startServer(api)
		})

		It("returns the initial password grant failure unclassified", func() {
			tokens.passwordStatus.Store(http.StatusUnauthorized)
			status := statusFor(apiURL, tokenURL, nil)
			_, _, err := factory.ServiceFor(ctx, newRealm("realm", "uid"), status)
			Expect(err).To(MatchError(ContainSubstring("retrieving token")))
			var ae *apiError
			Expect(errors.As(err, &ae)).To(BeFalse())
		})

		It("re-authenticates when the refresh token is rejected", func() {
			tokens.expiresIn.Store(1)
			tokens.refreshStatus.Store(http.StatusBadRequest)
			status := statusFor(apiURL, tokenURL, nil)
			lease, err := factory.acquire(ctx, newRealm("realm", "uid"), &status)
			Expect(err).NotTo(HaveOccurred())
			defer lease.Release()

			Expect(doGet(lease.Client())).To(Succeed())
			Expect(tokens.refreshCalls.Load()).To(BeEquivalentTo(1))
			Expect(tokens.passwordCalls.Load()).To(BeEquivalentTo(2))
			Expect(api.last()).To(Equal("Bearer token-password"))
		})

		DescribeTable("classifies refresh failures without re-authentication",
			func(status int, delay time.Duration) {
				tokens.expiresIn.Store(1)
				tokens.refreshStatus.Store(int32(status))
				realmStatus := statusFor(apiURL, tokenURL, nil)
				lease, err := factory.acquire(ctx, newRealm("realm", "uid"), &realmStatus)
				Expect(err).NotTo(HaveOccurred())
				defer lease.Release()

				err = doGet(lease.Client())
				var ae *apiError
				Expect(errors.As(err, &ae)).To(BeTrue(), "unexpected error %v", err)
				Expect(ae.IsRetryable()).To(BeTrue())
				Expect(ae.RetryDelay()).To(Equal(delay))
				Expect(tokens.passwordCalls.Load()).To(BeEquivalentTo(1))
			},
			Entry("rate limited", http.StatusTooManyRequests, 3*time.Second),
			Entry("server error", http.StatusServiceUnavailable, time.Duration(0)),
		)
	})
})

var _ = Describe("profile", func() {
	It("defaults to 10s, TLS 1.3 and the OAuth2 error classifier", func() {
		p, err := profile("")
		Expect(err).NotTo(HaveOccurred())
		Expect(p.Timeout).To(Equal(10 * time.Second))
		Expect(p.MinTLSVersion).To(BeEquivalentTo(tls.VersionTLS13))
		Expect(p.Transport).To(Equal(tlsclient.TransportRaw))
		Expect(p.MetricsClientName).To(Equal("identity"))
		Expect(p.ClassifyTokenError).NotTo(BeNil())
	})

	It("honours CLIENT_TIMEOUT", func() {
		p, err := profile("3s")
		Expect(err).NotTo(HaveOccurred())
		Expect(p.Timeout).To(Equal(3 * time.Second))

		_, err = profile("soon")
		Expect(err).To(MatchError(ContainSubstring("CLIENT_TIMEOUT")))
	})
})

var _ = Describe("ServiceFactoryFunc", func() {
	type ctxKey struct{}

	It("forwards all arguments and returns the results unchanged", func() {
		ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
		realm := &identityv1.Realm{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "ns"}}
		status := identityv1.RealmStatus{AdminUrl: "https://kc.example/admin"}
		svc := NewKeycloakService(nil)
		released := 0

		var gotCtx context.Context
		var gotRealm *identityv1.Realm
		var gotStatus identityv1.RealmStatus
		f := ServiceFactoryFunc(func(c context.Context, r *identityv1.Realm, s identityv1.RealmStatus) (KeycloakService, func(), error) {
			gotCtx, gotRealm, gotStatus = c, r, s
			return svc, func() { released++ }, nil
		})

		gotSvc, release, err := f.ServiceFor(ctx, realm, status)
		Expect(err).NotTo(HaveOccurred())
		Expect(gotCtx).To(BeIdenticalTo(ctx))
		Expect(gotRealm).To(BeIdenticalTo(realm))
		Expect(gotStatus).To(Equal(status))
		Expect(gotSvc).To(BeIdenticalTo(svc))
		Expect(release).NotTo(BeNil())
		release()
		Expect(released).To(Equal(1))
	})

	It("returns the error and accompanying values unchanged", func() {
		sentinel := errors.New("boom")
		wrapped := fmt.Errorf("acquiring: %w", sentinel)
		svc := NewKeycloakService(nil)
		released := 0
		f := ServiceFactoryFunc(func(context.Context, *identityv1.Realm, identityv1.RealmStatus) (KeycloakService, func(), error) {
			return svc, func() { released++ }, wrapped
		})

		gotSvc, release, err := f.ServiceFor(context.Background(), nil, identityv1.RealmStatus{})
		Expect(err).To(BeIdenticalTo(wrapped))
		Expect(errors.Is(err, sentinel)).To(BeTrue())
		Expect(gotSvc).To(BeIdenticalTo(svc))
		Expect(release).NotTo(BeNil())
		release()
		Expect(released).To(Equal(1))
	})
})
