// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/providerca"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/identity/api/endpoint"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/test/tlsutil"
	secrets "github.com/telekom/controlplane/secret-manager/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeKeycloak runs distinct token and admin API TLS servers.
type fakeKeycloak struct {
	token, api   *httptest.Server
	tokenCalls   atomic.Int32
	refreshCalls atomic.Int32
	apiAuth      atomic.Value
	expiresIn    int
	redirect     bool
}

func newFakeKeycloak(ca *tlsutil.CA) *fakeKeycloak {
	f := &fakeKeycloak{expiresIn: 300}
	var err error
	f.token, err = tlsutil.NewTLSServer(ca, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.redirect {
			http.Redirect(w, r, "https://example.com/elsewhere", http.StatusFound)
			return
		}
		Expect(r.ParseForm()).To(Succeed())
		n := f.tokenCalls.Add(1)
		if r.Form.Get("grant_type") == "refresh_token" {
			f.refreshCalls.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		Expect(json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("tok-%d", n), "token_type": "bearer",
			"expires_in": f.expiresIn, "refresh_token": "refresh",
		})).To(Succeed())
	}))
	Expect(err).NotTo(HaveOccurred())
	f.api, err = tlsutil.NewTLSServer(ca, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.apiAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(f.token.Close)
	DeferCleanup(f.api.Close)
	return f
}

func (f *fakeKeycloak) status(ref *ctypes.CABundleConfigMapRef) *identityv1.RealmStatus {
	s := &identityv1.RealmStatus{
		IssuerUrl:     f.token.URL + "/realms/test",
		AdminClientId: "admin-cli",
		AdminUserName: "admin",
		AdminPassword: "$<admin-secret>",
		AdminUrl:      f.api.URL + "/admin/realms/",
		AdminTokenUrl: f.token.URL + "/realms/master/protocol/openid-connect/token",
	}
	if ref != nil {
		s.TLS = &ctypes.ProviderTLS{CABundleConfigMapRef: ref}
	}
	return s
}

var factoryNs int

func newFactoryNamespace() string {
	factoryNs++
	name := fmt.Sprintf("factory-%d", factoryNs)
	Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
	return name
}

func putCA(ns, pemData string, labeled bool) *ctypes.CABundleConfigMapRef {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "ca", Namespace: ns}, Data: map[string]string{"ca.crt": pemData}}
	if labeled {
		cm.Labels = map[string]string{ctypes.ProviderCALabelKey: ctypes.ProviderCALabelValue}
	}
	Expect(k8sClient.Create(context.Background(), cm)).To(Succeed())
	return &ctypes.CABundleConfigMapRef{Name: "ca", Key: "ca.crt"}
}

func patchCA(ns string, mutate func(cm *corev1.ConfigMap)) {
	cm := &corev1.ConfigMap{}
	Expect(k8sClient.Get(context.Background(), k8stypes.NamespacedName{Namespace: ns, Name: "ca"}, cm)).To(Succeed())
	mutate(cm)
	Expect(k8sClient.Update(context.Background(), cm)).To(Succeed())
}

func createRealmWithStatus(ns, name string, status *identityv1.RealmStatus) *identityv1.Realm {
	ctx := context.Background()
	realm := &identityv1.Realm{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       identityv1.RealmSpec{IdentityProvider: &ctypes.ObjectRef{Namespace: ns, Name: "idp"}},
	}
	Expect(k8sClient.Create(ctx, realm)).To(Succeed())
	realm.Status = *status
	Expect(k8sClient.Status().Update(ctx, realm)).To(Succeed())
	return realm
}

// mutateStatus changes the in-memory Realm status passed to the factory.
func mutateStatus(realm *identityv1.Realm, mutate func(s *identityv1.RealmStatus)) {
	mutate(&realm.Status)
}

// countingBuilder wraps a stub service builder and records builds and idle closes.
type countingBuilder struct {
	builds, closes atomic.Int32
	onBuild        func(n int32)
}

func (b *countingBuilder) build(_, _ context.Context, _ *keycloak.Connection) (keycloak.KeycloakService, func(), error) {
	n := b.builds.Add(1)
	if b.onBuild != nil {
		b.onBuild(n)
	}
	return keycloak.NewKeycloakService(nil), func() { b.closes.Add(1) }, nil
}

var _ = Describe("Factory", func() {
	var (
		ns      string
		ca      *tlsutil.CA
		kc      *fakeKeycloak
		ctx     context.Context
		clock   time.Time
		clients *keycloak.ClientCache
	)

	BeforeEach(func() {
		ctx = context.Background()
		ns = newFactoryNamespace()
		var err error
		ca, err = tlsutil.NewCA("provider-ca")
		Expect(err).NotTo(HaveOccurred())
		kc = newFakeKeycloak(ca)

		original := secrets.Get
		secrets.Get = func(_ context.Context, ref string) (string, error) {
			if ref == "$<admin-secret>" {
				return "resolved", nil
			}
			return ref, nil
		}
		DeferCleanup(func() { secrets.Get = original })
	})

	// newFactory wires a factory to fresh shared caches whose CA expiry follows clock.
	newFactory := func(opts ...keycloak.FactoryOption) *keycloak.Factory {
		clock = time.Unix(1_700_000_000, 0)
		caCache := providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle, providerca.WithClock(func() time.Time { return clock }))
		clients = keycloak.NewClientCache(caCache)
		DeferCleanup(clients.Close)
		return keycloak.NewServiceFactory(clients, opts...)
	}
	expire := func() { clock = clock.Add(time.Minute) }

	Context("real TLS", func() {
		It("authenticates and calls the API through the configured CA", func() {
			ref := putCA(ns, ca.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(ref))

			svc, err := newFactory().ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(svc.DeleteRealm(ctx, "r")).To(Succeed())
			Expect(kc.apiAuth.Load()).To(Equal("Bearer tok-1"))

			By("not persisting the resolved password")
			realm := &identityv1.Realm{}
			Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Namespace: ns, Name: "r"}, realm)).To(Succeed())
			Expect(realm.Status.AdminPassword).To(Equal("$<admin-secret>"))
		})

		It("does not trust the provider CA with system trust", func() {
			owner := createRealmWithStatus(ns, "r", kc.status(nil))
			_, err := newFactory().ServiceFor(ctx, owner)
			Expect(err).To(MatchError(ContainSubstring("certificate")))
		})

		It("rejects a CA that did not issue the server certificate", func() {
			other, err := tlsutil.NewCA("other")
			Expect(err).NotTo(HaveOccurred())
			ref := putCA(ns, other.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(ref))
			_, err = newFactory().ServiceFor(ctx, owner)
			Expect(err).To(MatchError(ContainSubstring("certificate")))
		})

		It("rejects redirects on the credential-bearing token client", func() {
			kc.redirect = true
			ref := putCA(ns, ca.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(ref))
			_, err := newFactory().ServiceFor(ctx, owner)
			Expect(err).To(MatchError(ContainSubstring("redirects are not allowed")))
		})

		It("refreshes tokens with the factory context after the reconcile context is cancelled", func() {
			kc.expiresIn = 1 // below oauth2's expiry delta: next request refreshes
			ref := putCA(ns, ca.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(ref))

			reconcileCtx, cancel := context.WithCancel(ctx)
			svc, err := newFactory().ServiceFor(reconcileCtx, owner)
			Expect(err).NotTo(HaveOccurred())
			cancel()

			Expect(svc.DeleteRealm(ctx, "r")).To(Succeed())
			Expect(kc.refreshCalls.Load()).To(BeNumerically(">=", 1))
		})

		It("stops token refresh after the client cache is closed", func() {
			kc.expiresIn = 1
			ref := putCA(ns, ca.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(ref))
			f := newFactory()
			svc, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())

			clients.Close()
			Expect(svc.DeleteRealm(ctx, "r")).To(MatchError(ContainSubstring("canceled")))
			_, err = f.ServiceFor(ctx, owner)
			Expect(err).To(MatchError(ContainSubstring("closed")))
		})
	})

	Context("shared caches", func() {
		It("reuses the cached service while owner and trust are unchanged", func() {
			ref := putCA(ns, ca.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(ref))
			f := newFactory()
			a, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			b, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(b).To(BeIdenticalTo(a))
			Expect(kc.tokenCalls.Load()).To(Equal(int32(1)))
		})

		It("keeps the service after metadata-only ConfigMap changes are observed", func() {
			ref := putCA(ns, ca.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(ref))
			f := newFactory()
			a, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			patchCA(ns, func(cm *corev1.ConfigMap) { cm.Annotations = map[string]string{"irrelevant": "x"} })
			expire()
			b, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(b).To(BeIdenticalTo(a))
		})

		It("observes a lost provider-ca label once the CA cache expires", func() {
			ref := putCA(ns, ca.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(ref))
			f := newFactory()
			a, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())

			patchCA(ns, func(cm *corev1.ConfigMap) { cm.Labels = nil })
			b, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(b).To(BeIdenticalTo(a), "fresh cached trust is reused until it expires")

			expire()
			_, err = f.ServiceFor(ctx, owner)
			var be ctrlerrors.BlockedError
			Expect(errors.As(err, &be)).To(BeTrue())
			Expect(err).To(MatchError(ContainSubstring("lacks label")))
		})

		It("fails without a stale fallback once the CA ConfigMap is malformed or deleted", func() {
			ref := putCA(ns, ca.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(ref))
			f := newFactory()
			_, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())

			patchCA(ns, func(cm *corev1.ConfigMap) { cm.Data["ca.crt"] = "garbage" })
			expire()
			_, err = f.ServiceFor(ctx, owner)
			Expect(err).To(MatchError(ContainSubstring("invalid")))

			Expect(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "ca", Namespace: ns}})).To(Succeed())
			expire()
			_, err = f.ServiceFor(ctx, owner)
			Expect(err).To(MatchError(ContainSubstring("not found or lacks label")))
		})

		It("rebuilds after observed CA rotation and closes the replaced service", func() {
			ref := putCA(ns, ca.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(ref))
			b := &countingBuilder{}
			f := newFactory(keycloak.WithServiceBuilder(b.build))
			_, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())

			other, err := tlsutil.NewCA("rotated")
			Expect(err).NotTo(HaveOccurred())
			patchCA(ns, func(cm *corev1.ConfigMap) { cm.Data["ca.crt"] = ca.PEM + other.PEM })
			expire()
			_, err = f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(b.builds.Load()).To(Equal(int32(2)))
			Expect(b.closes.Load()).To(Equal(int32(1)))
		})

		It("rebuilds after credential changes", func() {
			owner := createRealmWithStatus(ns, "r", kc.status(nil))
			b := &countingBuilder{}
			f := newFactory(keycloak.WithServiceBuilder(b.build))
			_, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())

			mutateStatus(owner, func(s *identityv1.RealmStatus) { s.AdminUserName = "admin2" })
			_, err = f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(b.builds.Load()).To(Equal(int32(2)))
		})

		It("panics without its shared dependencies", func() {
			Expect(func() { keycloak.NewServiceFactory(nil) }).To(Panic())
		})

		It("rejects plaintext persisted endpoints with a custom CA before secrets, cache or network", func() {
			ref := putCA(ns, ca.PEM, true)
			b := &countingBuilder{}
			f := newFactory(keycloak.WithServiceBuilder(b.build))
			owners := map[string]*identityv1.Realm{}
			for _, field := range []string{"issuer", "admin", "token"} {
				st := kc.status(ref)
				switch field {
				case "issuer":
					st.IssuerUrl = "http://idp.example.com/realms/test"
				case "admin":
					st.AdminUrl = "http://idp.example.com/admin/realms/"
				case "token":
					st.AdminTokenUrl = "http://idp.example.com/realms/master/protocol/openid-connect/token"
				}
				owners[field] = createRealmWithStatus(ns, "plain-"+field, st)
			}
			original := secrets.Get
			secrets.Get = func(context.Context, string) (string, error) {
				Fail("no secret resolution expected")
				return "", nil
			}
			DeferCleanup(func() { secrets.Get = original })

			for field, owner := range owners {
				_, err := f.ServiceFor(ctx, owner)
				var be ctrlerrors.BlockedError
				Expect(errors.As(err, &be)).To(BeTrue(), field)
				Expect(err.Error()).To(ContainSubstring("must use https"), field)
			}
			Expect(b.builds.Load()).To(BeZero())
			Expect(kc.tokenCalls.Load()).To(BeZero())
		})

		It("rejects a historic snapshot modified to plaintext after it was cached", func() {
			ref := putCA(ns, ca.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(ref))
			b := &countingBuilder{}
			f := newFactory(keycloak.WithServiceBuilder(b.build))
			_, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())

			mutateStatus(owner, func(s *identityv1.RealmStatus) { s.AdminUrl = "http://idp.example.com/admin/realms/" })
			_, err = f.ServiceFor(ctx, owner)
			Expect(err).To(MatchError(ContainSubstring("must use https")))
			Expect(b.builds.Load()).To(Equal(int32(1)))
		})

		It("keeps plaintext compatibility for system-trust snapshots", func() {
			st := kc.status(nil)
			st.AdminUrl = "http://idp.example.com/admin/realms/"
			owner := createRealmWithStatus(ns, "r", st)
			b := &countingBuilder{}
			_, err := newFactory(keycloak.WithServiceBuilder(b.build)).ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(b.builds.Load()).To(Equal(int32(1)))
		})

		It("rebuilds for a recreated Realm with a new UID", func() {
			owner := createRealmWithStatus(ns, "r", kc.status(nil))
			b := &countingBuilder{}
			f := newFactory(keycloak.WithServiceBuilder(b.build))
			_, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			owner.UID = "other-uid"
			_, err = f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(b.builds.Load()).To(Equal(int32(2)))
			Expect(b.closes.Load()).To(Equal(int32(1)))
		})

		It("rebuilds after a target change and lets the previous service finish", func() {
			owner := createRealmWithStatus(ns, "r", kc.status(nil))
			b := &countingBuilder{}
			f := newFactory(keycloak.WithServiceBuilder(b.build))
			a, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())

			mutateStatus(owner, func(s *identityv1.RealmStatus) {
				s.AdminUrl = "https://other.example.com/admin/realms/"
				s.AdminTokenUrl = "https://other.example.com/realms/master/protocol/openid-connect/token"
			})
			c, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(c).NotTo(BeIdenticalTo(a))
			Expect(b.builds.Load()).To(Equal(int32(2)))
			Expect(b.closes.Load()).To(Equal(int32(1)), "only idle connections of the replaced service are closed")
		})

		It("rebuilds when the CA reference changes", func() {
			ref := putCA(ns, ca.PEM, true)
			owner := createRealmWithStatus(ns, "r", kc.status(nil))
			b := &countingBuilder{}
			f := newFactory(keycloak.WithServiceBuilder(b.build))
			_, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())

			mutateStatus(owner, func(s *identityv1.RealmStatus) { s.TLS = &ctypes.ProviderTLS{CABundleConfigMapRef: ref} })
			_, err = f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(b.builds.Load()).To(Equal(int32(2)))
		})

		It("resolves credential references at acquisition without changing the Realm", func() {
			owner := createRealmWithStatus(ns, "r", kc.status(nil))
			var got string
			f := newFactory(keycloak.WithServiceBuilder(func(_, _ context.Context, c *keycloak.Connection) (keycloak.KeycloakService, func(), error) {
				got = c.Password
				return keycloak.NewKeycloakService(nil), func() {}, nil
			}))
			_, err := f.ServiceFor(ctx, owner)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal("resolved"))
			Expect(owner.Status.AdminPassword).To(Equal("$<admin-secret>"))
		})

		It("rejects a nil Realm", func() {
			_, err := newFactory().ServiceFor(ctx, nil)
			Expect(err).To(MatchError(ContainSubstring("realm is nil")))
		})

		It("blocks without a complete snapshot", func() {
			owner := createRealmWithStatus(ns, "r", &identityv1.RealmStatus{AdminUrl: kc.api.URL})
			_, err := newFactory().ServiceFor(ctx, owner)
			var be ctrlerrors.BlockedError
			Expect(errors.As(err, &be)).To(BeTrue())
		})
	})
})

// validateCA resolves through a fresh shared CA cache so each call observes current state.
func validateCA(reader client.Reader, ns string, ref *ctypes.CABundleConfigMapRef) error {
	return keycloak.ValidateProviderCA(context.Background(), providerca.NewCache(reader, commonclient.ParseProviderCABundle), ns, ref)
}

var _ = Describe("ValidateProviderCA", func() {
	It("returns nil for system trust", func() {
		Expect(keycloak.ValidateProviderCA(context.Background(), nil, "default", nil)).To(Succeed())
	})

	It("blocks on a missing key", func() {
		ns := newFactoryNamespace()
		ca, err := tlsutil.NewCA("x")
		Expect(err).NotTo(HaveOccurred())
		putCA(ns, ca.PEM, true)
		err = validateCA(k8sClient, ns, &ctypes.CABundleConfigMapRef{Name: "ca", Key: "other"})
		Expect(err).To(MatchError(ContainSubstring("has no key")))
	})

	It("accepts an Admin-managed copy with complete provenance and picks up retained-copy rotation", func() {
		ns := newFactoryNamespace()
		caA, err := tlsutil.NewCA("a")
		Expect(err).NotTo(HaveOccurred())
		origin := providerca.ProviderCAOrigin{ZoneUID: "zuid", ZoneNamespace: "prod", ZoneName: "zone", ConfigMap: "src", Key: "ca.crt"}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: origin.CopyName(), Namespace: ns, Annotations: origin.Annotations(),
				Labels: map[string]string{
					ctypes.ProviderCALabelKey:    ctypes.ProviderCALabelValue,
					config.EnvironmentLabelKey:   "env",
					config.BuildLabelKey("zone"): "zone",
				},
			},
			Data: map[string]string{providerca.ProviderCACopyKey: caA.PEM},
		}
		Expect(k8sClient.Create(context.Background(), cm)).To(Succeed())
		ref := &ctypes.CABundleConfigMapRef{Name: cm.Name, Key: providerca.ProviderCACopyKey}
		Expect(validateCA(k8sClient, ns, ref)).To(Succeed())

		By("manually rotating the retained copy after Zone deletion")
		caB, err := tlsutil.NewCA("b")
		Expect(err).NotTo(HaveOccurred())
		cm.Data[providerca.ProviderCACopyKey] = caB.PEM
		Expect(k8sClient.Update(context.Background(), cm)).To(Succeed())
		Expect(validateCA(k8sClient, ns, ref)).To(Succeed())

		By("rejecting spoofed provenance without fallback")
		cm.Annotations[providerca.AnnotationProviderCAOriginZoneName] = "other"
		Expect(k8sClient.Update(context.Background(), cm)).To(Succeed())
		err = validateCA(k8sClient, ns, ref)
		Expect(err).To(MatchError(ContainSubstring("origin metadata")))
	})

	It("accepts ordinary labeled local sources but rejects partial copy metadata", func() {
		ns := newFactoryNamespace()
		ca, err := tlsutil.NewCA("x")
		Expect(err).NotTo(HaveOccurred())
		ref := putCA(ns, ca.PEM, true)
		err = validateCA(k8sClient, ns, ref)
		Expect(err).NotTo(HaveOccurred())

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: ref.Name}, cm)).To(Succeed())
		cm.Annotations = map[string]string{providerca.AnnotationProviderCAOriginConfigMap: "src"}
		Expect(k8sClient.Update(context.Background(), cm)).To(Succeed())
		err = validateCA(k8sClient, ns, ref)
		Expect(err).To(MatchError(ContainSubstring("origin metadata")))
	})

	It("rejects unlabeled ConfigMaps returned by an unfiltered reader", func() {
		ns := newFactoryNamespace()
		ca, err := tlsutil.NewCA("x")
		Expect(err).NotTo(HaveOccurred())
		ref := putCA(ns, ca.PEM, false)
		err = validateCA(k8sClient, ns, ref)
		Expect(err).To(MatchError(ContainSubstring("lacks label")))
	})

	It("maps shared resolver failures to Identity controller errors", func() {
		isBlocked := func(err error) bool {
			var ce *ctrlerrors.CtrlError
			return errors.As(err, &ce) && ce.IsBlocked()
		}
		ns := newFactoryNamespace()

		err := validateCA(k8sClient, ns, &ctypes.CABundleConfigMapRef{Name: "Bad_Name", Key: "k"})
		Expect(isBlocked(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring(`invalid caBundleConfigMapRef name "Bad_Name"`))

		err = validateCA(k8sClient, ns, &ctypes.CABundleConfigMapRef{Name: "ca", Key: "ca.crt"})
		Expect(isBlocked(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("CA bundle ConfigMap %s/ca not found or lacks label", ns)))

		ref := putCA(ns, "garbage", true)
		err = validateCA(k8sClient, ns, ref)
		Expect(isBlocked(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring(fmt.Sprintf(`CA bundle ConfigMap %s/ca key "ca.crt" is invalid:`, ns)))
	})

	It("maps forbidden and transient reads to Identity controller errors", func() {
		ns := newFactoryNamespace()
		ref := &ctypes.CABundleConfigMapRef{Name: "ca", Key: "ca.crt"}

		restricted := rest.CopyConfig(testEnv.Config)
		restricted.Impersonate = rest.ImpersonationConfig{UserName: "resolve-no-rbac"}
		forbiddenReader, err := client.New(restricted, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		err = validateCA(forbiddenReader, ns, ref)
		var ce *ctrlerrors.CtrlError
		Expect(errors.As(err, &ce)).To(BeTrue(), "error %v", err)
		Expect(ce.IsBlocked()).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("CA bundle ConfigMap %s/ca is not available: Forbidden", ns)))

		unreachable, err := client.New(&rest.Config{Host: "http://127.0.0.1:1", Timeout: time.Second},
			client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		err = validateCA(unreachable, ns, ref)
		ce = nil
		Expect(errors.As(err, &ce)).To(BeTrue(), "error %v", err)
		Expect(ce.IsRetryable()).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("reading CA bundle ConfigMap %s/ca: ", ns)))
	})
})

var _ = Describe("Keycloak endpoint derivation", func() {
	DescribeTable("derives issuer and admin-token URLs from the admin URL",
		func(adminURL, realm, issuer, token string) {
			Expect(endpoint.IssuerURL(adminURL, realm)).To(Equal(issuer))
			Expect(endpoint.TokenURL(adminURL, realm)).To(Equal(token))
			Expect(keycloak.DetermineIssuerUrlFrom(adminURL, realm)).To(Equal(issuer))
			Expect(keycloak.DetermineAdminTokenUrlFrom(adminURL, realm)).To(Equal(token))
		},
		Entry("splits at /admin", "https://h/auth/admin/realms/", "r",
			"https://h/auth/realms/r", "https://h/auth/realms/r/protocol/openid-connect/token"),
		Entry("master realm", "https://h/auth/admin/realms/", keycloak.MasterRealm,
			"https://h/auth/realms/master", "https://h/auth/realms/master/protocol/openid-connect/token"),
		Entry("splits at the first /admin", "https://h/admin/x/admin/", "r",
			"https://h/realms/r", "https://h/realms/r/protocol/openid-connect/token"),
		Entry("appends without /admin", "https://h/auth/", "r",
			"https://h/auth/realms/r", "https://h/auth/realms/r/protocol/openid-connect/token"),
		Entry("appends verbatim without trailing slash", "https://h", "r",
			"https://hrealms/r", "https://hrealms/r/protocol/openid-connect/token"),
	)

	It("keeps the forwarding constants", func() {
		Expect(keycloak.MasterRealm).To(Equal(endpoint.MasterRealm))
		Expect(keycloak.TokenEndpointSuffix).To(Equal(endpoint.TokenEndpointSuffix))
	})
})

var _ = Describe("Snapshot helpers", func() {
	It("classifies snapshots", func() {
		Expect(identityv1.ClassifyRealmSnapshot(&identityv1.RealmStatus{})).To(Equal(identityv1.RealmSnapshotEmpty))
		Expect(identityv1.ClassifyRealmSnapshot(&identityv1.RealmStatus{
			TLS: &ctypes.ProviderTLS{CABundleConfigMapRef: &ctypes.CABundleConfigMapRef{Name: "a", Key: "b"}},
		})).To(Equal(identityv1.RealmSnapshotPartial))
		complete := &identityv1.RealmStatus{IssuerUrl: "a", AdminUrl: "b", AdminTokenUrl: "c", AdminClientId: "d", AdminUserName: "e", AdminPassword: "f"}
		Expect(identityv1.ClassifyRealmSnapshot(complete)).To(Equal(identityv1.RealmSnapshotComplete))
	})

	It("requires HTTPS", func() {
		Expect(keycloak.RequireHTTPS("https://a", "HTTPS://b")).To(Succeed())
		Expect(keycloak.RequireHTTPS("http://a")).To(HaveOccurred())
	})
})
