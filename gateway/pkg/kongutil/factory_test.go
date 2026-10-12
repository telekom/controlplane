// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ptls "github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/providerca"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	kongclient "github.com/telekom/controlplane/gateway/pkg/kong/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const ns = "zone-ns"

// tokenHandler counts grants and issues tokens whose lifetime is read from expiresIn.
// oauth2 treats tokens expiring within 10 seconds as expired, so expiresIn=1 forces a new grant
// on the next use without waiting.
func tokenHandler(calls, expiresIn *atomic.Int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != tokenURLPath {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer") {
			http.Error(w, "access token leaked to token request", http.StatusBadRequest)
			return
		}
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("tok-%d", n), "token_type": "Bearer", "expires_in": expiresIn.Load(),
		})
	})
}

// testConsumer is a Kong consumer identified by its name.
type testConsumer string

func (testConsumer) SetId(string)              {}
func (c testConsumer) GetConsumerName() string { return string(c) }

func apiHandler(calls *atomic.Int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tok-") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/consumers/redirect" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func caConfigMap(name, data string, labeled bool) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: ktypes.UID("uid-" + name)},
		Data:       map[string]string{"ca.crt": data},
	}
	if labeled {
		cm.Labels = map[string]string{ctypes.ProviderCALabelKey: ctypes.ProviderCALabelValue}
	}
	return cm
}

func adminCopy(origin *providerca.ProviderCAOrigin, data string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: origin.CopyName(), Namespace: ns, UID: ktypes.UID("uid-copy"), Annotations: origin.Annotations(),
			Labels: map[string]string{
				ctypes.ProviderCALabelKey:    ctypes.ProviderCALabelValue,
				config.EnvironmentLabelKey:   "env",
				config.BuildLabelKey("zone"): origin.ZoneName,
			},
		},
		Data: map[string]string{providerca.ProviderCACopyKey: data},
	}
}

func ref(name string) *ctypes.ProviderTLS {
	return &ctypes.ProviderTLS{CABundleConfigMapRef: &ctypes.CABundleConfigMapRef{Name: name, Key: "ca.crt"}}
}

func newGW(name, apiURL, issuerURL string) *gatewayv1.Gateway {
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: ktypes.UID("uid-" + name)},
		Spec: gatewayv1.GatewaySpec{Admin: gatewayv1.AdminConfig{
			Url: apiURL, IssuerUrl: issuerURL, ClientId: "cid", ClientSecret: "secret-ref",
		}},
	}
}

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	Expect(corev1.AddToScheme(s)).To(Succeed())
	Expect(gatewayv1.AddToScheme(s)).To(Succeed())
	return s
}

var _ = Describe("Factory", func() {
	var (
		ctx             context.Context
		caAPI, caIssuer *testCA
		apiSrvURL       string
		issuerURL       string
		tokenCalls      atomic.Int64
		apiCalls        atomic.Int64
		secretCalls     atomic.Int64
		expiresIn       atomic.Int64
		k8s             client.Client
		clock           time.Time
		clients         *ClientCache
		factory         *Factory
	)

	BeforeEach(func() {
		ctx = context.Background()
		tokenCalls.Store(0)
		apiCalls.Store(0)
		secretCalls.Store(0)
		expiresIn.Store(3600)
		caAPI = newTestCA("api-ca")
		caIssuer = newTestCA("issuer-ca")
		apiSrvURL = caAPI.serve(apiHandler(&apiCalls)).URL
		issuerURL = caIssuer.serve(tokenHandler(&tokenCalls, &expiresIn)).URL
		k8s = fake.NewClientBuilder().WithScheme(newScheme()).Build()
		clock = time.Unix(1_700_000_000, 0)
		ca := providerca.NewCache(k8s, ptls.ParseProviderCABundle, providerca.WithClock(func() time.Time { return clock }))
		clients = NewClientCache(ca)
		DeferCleanup(clients.Close)
		factory = NewFactory(ca, clients, func(_ context.Context, r string) (string, error) {
			secretCalls.Add(1)
			return "resolved-" + r, nil
		})
	})

	createGW := func(name string, tls *ctypes.ProviderTLS) *gatewayv1.Gateway {
		gw := newGW(name, apiSrvURL, issuerURL)
		gw.Spec.Admin.TLS = tls
		Expect(k8s.Create(ctx, gw)).To(Succeed())
		return gw
	}

	// call performs a Kong API request deleting the named consumer.
	call := func(kc kongclient.KongClient, name string) error {
		return kc.DeleteConsumer(context.Background(), testConsumer(name))
	}

	Context("TLS", func() {
		createZoneCA := func() {
			Expect(k8s.Create(ctx, caConfigMap("zone-ca", caAPI.pem+caIssuer.pem, true))).To(Succeed())
		}

		It("reaches both endpoints through one combined bundle and one transport", func() {
			createZoneCA()
			kc, err := factory.ClientFor(ctx, createGW("gw", ref("zone-ca")))
			Expect(err).NotTo(HaveOccurred())
			Expect(call(kc, "c")).To(Succeed())
			Expect(tokenCalls.Load()).To(BeEquivalentTo(1), "the token request must not carry the access token")
			Expect(apiCalls.Load()).To(BeEquivalentTo(1))
		})

		It("fails closed when the bundle lacks the issuer CA", func() {
			Expect(k8s.Create(ctx, caConfigMap("api-ca", caAPI.pem, true))).To(Succeed())
			kc, err := factory.ClientFor(ctx, createGW("gw", ref("api-ca")))
			Expect(err).NotTo(HaveOccurred())
			err = call(kc, "c")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("certificate"))
			Expect(tokenCalls.Load()).To(BeZero())
			Expect(apiCalls.Load()).To(BeZero())
		})

		It("rejects redirects", func() {
			createZoneCA()
			gw := createGW("gw", ref("zone-ca"))
			kc, err := factory.ClientFor(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			err = call(kc, "redirect")
			Expect(err).To(MatchError(ContainSubstring(ptls.ErrRedirectNotAllowed.Error())))
			Expect(apiCalls.Load()).To(BeEquivalentTo(1))
		})

		It("reuses a valid token without a new grant", func() {
			createZoneCA()
			kc, err := factory.ClientFor(ctx, createGW("gw", ref("zone-ca")))
			Expect(err).NotTo(HaveOccurred())
			Expect(call(kc, "c")).To(Succeed())
			Expect(call(kc, "c")).To(Succeed())
			Expect(tokenCalls.Load()).To(BeEquivalentTo(1))
		})

		It("refreshes an expired token with the manager context after the construction context ends", func() {
			createZoneCA()
			gw := createGW("gw", ref("zone-ca"))
			expiresIn.Store(1) // every token is already expired for oauth2's 10s delta

			rctx, cancel := context.WithCancel(ctx)
			kc, err := factory.ClientFor(rctx, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(call(kc, "c")).To(Succeed())
			Expect(tokenCalls.Load()).To(BeEquivalentTo(1))

			cancel()
			Expect(call(kc, "c")).To(Succeed())
			Expect(tokenCalls.Load()).To(BeEquivalentTo(2), "expired token must trigger a second grant over the shared transport")
			Expect(apiCalls.Load()).To(BeEquivalentTo(2))

			clients.Close()
			err = call(kc, "c")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
			Expect(tokenCalls.Load()).To(BeEquivalentTo(2))
			Expect(apiCalls.Load()).To(BeEquivalentTo(2))
		})
	})

	Context("custom trust endpoints", func() {
		expectBlockedEndpoint := func(gw *gatewayv1.Gateway) {
			_, err := factory.ClientFor(ctx, gw)
			te := AsTrustError(err)
			Expect(te).To(HaveOccurred(), "error %v", err)
			Expect(te.Reason).To(Equal("InvalidEndpoint"))
			Expect(AsTrustError(factory.Validate(ctx, gw))).To(HaveOccurred())
			Expect(apiCalls.Load()).To(BeZero())
			Expect(tokenCalls.Load()).To(BeZero())
			Expect(secretCalls.Load()).To(BeZero())
		}

		BeforeEach(func() {
			Expect(k8s.Create(ctx, caConfigMap("api-ca", caAPI.pem+caIssuer.pem, true))).To(Succeed())
		})

		DescribeTable("rejects non-https API URLs with custom trust",
			func(rawURL string) {
				gw := newGW("gw", rawURL, issuerURL)
				gw.Spec.Admin.TLS = ref("api-ca")
				Expect(k8s.Create(ctx, gw)).To(Succeed())
				expectBlockedEndpoint(gw)
			},
			Entry("http", "http://127.0.0.1:1/"),
			Entry("relative", "/admin"),
			Entry("no hostname", "https:///admin"),
			Entry("userinfo", "https://user:pw@127.0.0.1:1/"),
			Entry("fragment", "https://127.0.0.1:1/#frag"),
			Entry("empty fragment marker", "https://127.0.0.1:1/#"),
		)

		DescribeTable("rejects non-https issuer URLs with custom trust",
			func(rawURL string) {
				gw := newGW("gw", apiSrvURL, rawURL)
				gw.Spec.Admin.TLS = ref("api-ca")
				Expect(k8s.Create(ctx, gw)).To(Succeed())
				expectBlockedEndpoint(gw)
			},
			Entry("http", "http://127.0.0.1:1/realms/x"),
			Entry("userinfo", "https://u@127.0.0.1:1/realms/x"),
			Entry("fragment", "https://127.0.0.1:1/realms/x#f"),
		)

		It("keeps nil references compatible with http URLs", func() {
			gw := newGW("gw", "http://127.0.0.1:1/", "http://127.0.0.1:1/realms/x")
			Expect(k8s.Create(ctx, gw)).To(Succeed())
			_, err := factory.ClientFor(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("validation", func() {
		expectTrustErr := func(gw *gatewayv1.Gateway, reason string) {
			_, err := factory.ClientFor(ctx, gw)
			te := AsTrustError(err)
			Expect(te).To(HaveOccurred(), "error %v", err)
			Expect(te.Reason).To(Equal(reason))
		}

		It("blocks a missing ConfigMap without falling back to system trust", func() {
			gw := createGW("gw", ref("missing"))
			expectTrustErr(gw, "ConfigMapNotFound")
			_, err := factory.ClientFor(ctx, gw)
			var be ctrlerrors.BlockedError
			Expect(errors.As(err, &be)).To(BeTrue())
			Expect(be.IsBlocked()).To(BeTrue())
		})

		It("blocks an unlabeled source", func() {
			Expect(k8s.Create(ctx, caConfigMap("api-ca", caAPI.pem, false))).To(Succeed())
			expectTrustErr(createGW("gw", ref("api-ca")), "LabelMissing")
		})

		It("accepts an ordinary labeled local source without provenance", func() {
			Expect(k8s.Create(ctx, caConfigMap("manual-ca", caAPI.pem, true))).To(Succeed())
			Expect(factory.Validate(ctx, createGW("gw", ref("manual-ca")))).To(Succeed())
		})

		It("blocks a copy-named ConfigMap without complete provenance", func() {
			origin := providerca.ProviderCAOrigin{ZoneUID: "zuid", ZoneNamespace: "prod", ZoneName: "zone", ConfigMap: "src", Key: "ca.crt"}
			cm := adminCopy(&origin, caAPI.pem)
			delete(cm.Annotations, providerca.AnnotationProviderCAOriginHash)
			Expect(k8s.Create(ctx, cm)).To(Succeed())
			expectTrustErr(createGW("gw", ref(cm.Name)), "InvalidProvenance")
		})

		It("blocks a local source carrying partial copy metadata", func() {
			cm := caConfigMap("api-ca", caAPI.pem, true)
			cm.Annotations = map[string]string{providerca.AnnotationZoneUID: "zuid"}
			Expect(k8s.Create(ctx, cm)).To(Succeed())
			expectTrustErr(createGW("gw", ref("api-ca")), "InvalidProvenance")
		})

		It("blocks a missing key", func() {
			cm := caConfigMap("api-ca", caAPI.pem, true)
			cm.Data = map[string]string{"other": caAPI.pem}
			Expect(k8s.Create(ctx, cm)).To(Succeed())
			expectTrustErr(createGW("gw", ref("api-ca")), "KeyMissing")
			Expect(factory.Validate(ctx, createGW("gw2", ref("api-ca")))).
				To(MatchError(fmt.Sprintf(`CA ConfigMap %s/api-ca has no key "ca.crt"`, ns)))
		})

		It("blocks an invalid bundle", func() {
			Expect(k8s.Create(ctx, caConfigMap("api-ca", "garbage", true))).To(Succeed())
			expectTrustErr(createGW("gw", ref("api-ca")), "InvalidBundle")
		})

		It("blocks an invalid reference", func() {
			gw := createGW("gw", &ctypes.ProviderTLS{CABundleConfigMapRef: &ctypes.CABundleConfigMapRef{Name: "Bad_Name", Key: "k"}})
			expectTrustErr(gw, "InvalidReference")
		})

		It("rejects a nil gateway", func() {
			_, err := factory.ClientFor(ctx, nil)
			Expect(err).To(HaveOccurred())
		})

		It("validates without building", func() {
			Expect(k8s.Create(ctx, caConfigMap("api-ca", caAPI.pem, true))).To(Succeed())
			Expect(factory.Validate(ctx, createGW("gw", ref("api-ca")))).To(Succeed())
			Expect(secretCalls.Load()).To(BeZero())
			Expect(tokenCalls.Load()).To(BeZero())
		})
	})

	Context("shared caches", func() {
		It("reuses the client for unchanged inputs and resolves the secret on every acquisition", func() {
			gw := createGW("gw", nil)
			a, err := factory.ClientFor(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			b, err := factory.ClientFor(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(b).To(BeIdenticalTo(a))
			Expect(secretCalls.Load()).To(BeEquivalentTo(2))
		})

		It("replaces the client when the Gateway connection or secret changes", func() {
			gw := createGW("gw", nil)
			a, err := factory.ClientFor(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			gw.Spec.Admin.ClientSecret = "rotated"
			b, err := factory.ClientFor(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(b).NotTo(BeIdenticalTo(a))
			gw.Spec.Admin.IssuerUrl = issuerURL + "/other"
			c, err := factory.ClientFor(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(c).NotTo(BeIdenticalTo(b))
		})

		It("observes trust changes in validation and acquisition only after the CA cache expires", func() {
			cm := caConfigMap("api-ca", caAPI.pem, true)
			Expect(k8s.Create(ctx, cm)).To(Succeed())
			gw := createGW("gw", ref("api-ca"))
			a, err := factory.ClientFor(ctx, gw)
			Expect(err).NotTo(HaveOccurred())

			cm.Labels = nil
			Expect(k8s.Update(ctx, cm)).To(Succeed())
			Expect(factory.Validate(ctx, gw)).To(Succeed())
			b, err := factory.ClientFor(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(b).To(BeIdenticalTo(a))

			clock = clock.Add(time.Minute)
			Expect(AsTrustError(factory.Validate(ctx, gw)).Reason).To(Equal("LabelMissing"))
			_, err = factory.ClientFor(ctx, gw)
			Expect(AsTrustError(err).Reason).To(Equal("LabelMissing"))
		})

		It("replaces the client after CA rotation is observed", func() {
			cm := caConfigMap("api-ca", caAPI.pem, true)
			Expect(k8s.Create(ctx, cm)).To(Succeed())
			gw := createGW("gw", ref("api-ca"))
			a, err := factory.ClientFor(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			cm.Data["ca.crt"] = caAPI.pem + caIssuer.pem
			Expect(k8s.Update(ctx, cm)).To(Succeed())
			clock = clock.Add(time.Minute)
			b, err := factory.ClientFor(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(b).NotTo(BeIdenticalTo(a))
		})

		It("blocks acquisition of a missing CA before contacting any endpoint", func() {
			_, err := factory.ClientFor(ctx, createGW("gw", ref("missing")))
			Expect(AsTrustError(err).Reason).To(Equal("ConfigMapNotFound"))
			Expect(err.Error()).To(Equal(fmt.Sprintf("CA ConfigMap %s/missing not found or lacks label cp.ei.telekom.de/provider-ca=true", ns)))
			Expect(tokenCalls.Load()).To(BeZero())
			Expect(apiCalls.Load()).To(BeZero())
		})

		It("panics without its shared dependencies", func() {
			Expect(func() { NewFactory(nil, clients, nil) }).To(Panic())
		})
	})
})
