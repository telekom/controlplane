// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	serverclient "github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/providerca"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newTestCAPEM(cn string) string {
	GinkgoHelper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func canonical(p string) string {
	GinkgoHelper()
	b, err := serverclient.ParseProviderCABundle([]byte(p))
	Expect(err).NotTo(HaveOccurred())
	return string(b.CanonicalPEM())
}

func putSource(name, data string, labeled bool) {
	GinkgoHelper()
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace}}
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
	if err != nil {
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}
	cm.Labels = map[string]string{}
	if labeled {
		cm.Labels[types.ProviderCALabelKey] = types.ProviderCALabelValue
	}
	cm.Data = map[string]string{"ca.crt": data}
	if err != nil {
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		return
	}
	Expect(k8sClient.Update(ctx, cm)).To(Succeed())
}

func deleteSource(name string) {
	GinkgoHelper()
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace}}
	Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cm))).To(Succeed())
}

func tlsRef(name string) *types.ProviderTLS {
	return &types.ProviderTLS{CABundleConfigMapRef: &types.CABundleConfigMapRef{Name: name, Key: "ca.crt"}}
}

// getCopy reads the durable CA bundle of zone.
func getCopy(zone *adminv1.Zone) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	o := zoneOrigin(zone)
	err := k8sClient.Get(ctx, client.ObjectKey{Namespace: zoneNamespace(zone), Name: o.CopyName()}, cm)
	return cm, err
}

func getIDP(zone *adminv1.Zone) *identityapi.IdentityProvider {
	GinkgoHelper()
	idp := &identityapi.IdentityProvider{}
	Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForIdentityProvider(zone, "primary")}, idp)).To(Succeed())
	return idp
}

func getGateway(zone *adminv1.Zone) *gatewayapi.Gateway {
	GinkgoHelper()
	gw := &gatewayapi.Gateway{}
	Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForGateway(zone, "standard")}, gw)).To(Succeed())
	return gw
}

func isBlocked(err error) bool {
	var ce *ctrlerrors.CtrlError
	return errors.As(err, &ce) && ce.IsBlocked()
}

func newTrustHandler() *ZoneHandler {
	useSecretManager()
	return NewZoneHandler(k8sClient, k8sClient)
}

var _ = Describe("Zone trust helpers", func() {
	DescribeTable("normalizes target URLs",
		func(in, out string) {
			got, err := normalizeURL(in)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(out))
		},
		Entry("default port and case", "HTTPS://Example.COM:443", "https://example.com/"),
		Entry("keeps path, slash and query", "https://a.de:8443/auth/admin/realms/?x=1", "https://a.de:8443/auth/admin/realms/?x=1"),
		Entry("keeps escaped path", "https://a.de/a%2Fb", "https://a.de/a%2Fb"),
		Entry("keeps empty query marker", "https://a.de/a?", "https://a.de/a?"),
		Entry("drops default http port", "http://a.de:80/x", "http://a.de/x"),
		Entry("keeps 443 for http", "http://a.de:443/x", "http://a.de:443/x"),
		Entry("brackets IPv6", "https://[FE80::1]:443/x", "https://[fe80::1]/x"),
		Entry("brackets IPv6 with port", "https://[::1]:8443/x", "https://[::1]:8443/x"),
	)

	It("rejects an empty hostname even with a port", func() {
		_, err := normalizeURL("https://:8443/x")
		Expect(err).To(HaveOccurred())
	})

	It("requires https case-insensitively", func() {
		Expect(requireHTTPS("HTTPS://a.de/")).To(Succeed())
		Expect(requireHTTPS("http://a.de/")).To(MatchError(ContainSubstring("must use https when a CA bundle is configured")))
		Expect(requireHTTPS("://bad")).To(HaveOccurred())
	})

	It("requires a reader and a writer", func() {
		Expect(func() { NewZoneHandler(nil, k8sClient) }).To(PanicWith("zone.NewZoneHandler: reader and writer are required"))
		Expect(func() { NewZoneHandler(k8sClient, nil) }).To(PanicWith("zone.NewZoneHandler: reader and writer are required"))
		Expect(NewZoneHandler(k8sClient, k8sClient).Reader).To(BeIdenticalTo(k8sClient))
	})

	It("maps shared resolver failures to Admin source errors", func() {
		h := NewZoneHandler(k8sClient, k8sClient)
		_, err := h.readSource(ctx, testNamespace, &types.CABundleConfigMapRef{Name: "Bad_Name", Key: "ca.crt"})
		Expect(isBlocked(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("provider CA source default/Bad_Name: invalid caBundleConfigMapRef name"))

		_, err = h.readSource(ctx, testNamespace, &types.CABundleConfigMapRef{Name: "readsource-missing", Key: "ca.crt"})
		Expect(isBlocked(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("provider CA source default/readsource-missing not found or lacks label"))

		putSource("readsource-unlabeled", newTestCAPEM("x"), false)
		DeferCleanup(deleteSource, "readsource-unlabeled")
		_, err = h.readSource(ctx, testNamespace, &types.CABundleConfigMapRef{Name: "readsource-unlabeled", Key: "ca.crt"})
		Expect(isBlocked(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("provider CA source default/readsource-unlabeled lacks label"))

		putSource("readsource-ca", "garbage", true)
		DeferCleanup(deleteSource, "readsource-ca")
		_, err = h.readSource(ctx, testNamespace, &types.CABundleConfigMapRef{Name: "readsource-ca", Key: "other"})
		Expect(isBlocked(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring(`provider CA source default/readsource-ca has no key "other"`))
		_, err = h.readSource(ctx, testNamespace, &types.CABundleConfigMapRef{Name: "readsource-ca", Key: "ca.crt"})
		Expect(isBlocked(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring(`provider CA source default/readsource-ca key "ca.crt" is invalid`))

		pemData := newTestCAPEM("ok")
		putSource("readsource-ca", pemData, true)
		bundle, err := h.readSource(ctx, testNamespace, &types.CABundleConfigMapRef{Name: "readsource-ca", Key: "ca.crt"})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(bundle.CanonicalPEM())).To(Equal(canonical(pemData)))
	})

	It("maps forbidden and transient source reads to Admin errors", func() {
		ref := &types.CABundleConfigMapRef{Name: "readsource-ca", Key: "ca.crt"}

		restricted := rest.CopyConfig(testEnv.Config)
		restricted.Impersonate = rest.ImpersonationConfig{UserName: "readsource-no-rbac"}
		forbiddenReader, err := client.New(restricted, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		_, err = (&ZoneHandler{Reader: forbiddenReader}).readSource(ctx, testNamespace, ref)
		Expect(isBlocked(err)).To(BeTrue(), "error %v", err)
		Expect(err.Error()).To(ContainSubstring("provider CA source default/readsource-ca is forbidden"))

		unreachable, err := client.New(&rest.Config{Host: "http://127.0.0.1:1", Timeout: time.Second},
			client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		_, err = (&ZoneHandler{Reader: unreachable}).readSource(ctx, testNamespace, ref)
		var ce *ctrlerrors.CtrlError
		Expect(errors.As(err, &ce)).To(BeTrue(), "error %v", err)
		Expect(ce.IsRetryable()).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("reading provider CA source default/readsource-ca: "))
	})

	It("rejects userinfo and fragments", func() {
		_, err := normalizeURL("https://u:p@a.de/")
		Expect(err).To(HaveOccurred())
		_, err = normalizeURL("https://a.de/#x")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("OIDC discovery with provider trust", func() {
	var server *httptest.Server

	BeforeEach(func() {
		mux := http.NewServeMux()
		server = httptest.NewTLSServer(mux)
		issuer := server.URL + "/auth/realms/r"
		mux.HandleFunc("/auth/realms/r/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
			Expect(json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "token_endpoint": issuer + "/token"})).To(Succeed())
		})
		mux.HandleFunc("/redirect/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/auth/realms/r/.well-known/openid-configuration", http.StatusFound)
		})
		DeferCleanup(server.Close)
	})

	serverBundle := func() *serverclient.ProviderCABundle {
		p := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
		b, err := serverclient.ParseProviderCABundle(p)
		Expect(err).NotTo(HaveOccurred())
		return b
	}

	It("discovers through the effective custom trust", func() {
		c, err := newDiscoveryClient(serverBundle())
		Expect(err).NotTo(HaveOccurred())
		tokenURL, err := discoverTokenURL(ctx, c, server.URL+"/auth/realms/r")
		Expect(err).NotTo(HaveOccurred())
		Expect(tokenURL).To(Equal(server.URL + "/auth/realms/r/token"))
	})

	It("fails under system trust", func() {
		c, err := newDiscoveryClient(nil)
		Expect(err).NotTo(HaveOccurred())
		_, err = discoverTokenURL(ctx, c, server.URL+"/auth/realms/r")
		Expect(err).To(MatchError(ContainSubstring("certificate")))
	})

	It("denies redirects", func() {
		c, err := newDiscoveryClient(serverBundle())
		Expect(err).NotTo(HaveOccurred())
		_, err = discoverTokenURL(ctx, c, server.URL+"/redirect")
		Expect(err).To(MatchError(ContainSubstring("redirects are not allowed")))
	})
})

var _ = Describe("Zone provider CA trust", func() {
	var zone *adminv1.Zone
	var source string
	var idx int

	BeforeEach(func() {
		idx++
		source = fmt.Sprintf("provider-ca-src-%d", idx)
		zone = newTestZone(fmt.Sprintf("trust-zone-%d", idx))
	})

	create := func() {
		GinkgoHelper()
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed()) })
		DeferCleanup(func() { deleteSource(source) })
	}

	run := func(h *ZoneHandler) error {
		return h.CreateOrUpdate(newTestContext(zone), zone)
	}

	provisionWithTrust := func(h *ZoneHandler) {
		GinkgoHelper()
		Expect(run(h)).To(Succeed())
		markSubResourcesReady(zone)
		Expect(run(h)).To(Succeed())
	}

	trustCondition := func() *metav1.Condition {
		return meta.FindStatusCondition(zone.Status.Conditions, ConditionTypeTrustSourcesReady)
	}

	It("materializes one durable Zone bundle for a shared source and publishes it to all providers", func() {
		pemA := newTestCAPEM("a")
		putSource(source, pemA, true)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		zone.Spec.Gateways[0].TLS = tlsRef(source)
		create()
		h := newTrustHandler()
		provisionWithTrust(h)

		cp, err := getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(cp.Labels).To(HaveKeyWithValue(types.ProviderCALabelKey, "true"))
		Expect(cp.Labels).To(HaveKeyWithValue(config.EnvironmentLabelKey, testEnvironment))
		Expect(cp.Labels).To(HaveKeyWithValue(config.BuildLabelKey(zoneLabelName), zone.Name))
		Expect(cp.Labels).To(HaveKeyWithValue(config.DomainLabelKey, domainName))
		Expect(cp.Annotations).To(HaveKeyWithValue(providerca.AnnotationZoneUID, string(zone.UID)))
		Expect(cp.Annotations).NotTo(HaveKey(providerca.AnnotationProviderCAOriginConfigMap), "a Zone bundle claims no single source")
		Expect(cp.Annotations).NotTo(HaveKey(providerca.AnnotationProviderCAOriginKey))
		Expect(cp.OwnerReferences).To(BeEmpty())
		Expect(cp.Data).To(Equal(map[string]string{"ca.crt": canonical(pemA)}))

		want := &types.CABundleConfigMapRef{Name: cp.Name, Key: providerca.ProviderCACopyKey}
		idp := getIDP(zone)
		Expect(idp.Spec.TLS.Ref()).To(Equal(want))
		Expect(idp.Annotations).To(HaveKeyWithValue(providerca.AnnotationZoneUID, string(zone.UID)))
		Expect(idp.Annotations).To(HaveKeyWithValue(AnnotationDiscoveryIssuer, "https://test-iris.de/"))
		gw := getGateway(zone)
		Expect(gw.Spec.Admin.TLS.Ref()).To(Equal(want))

		cond := trustCondition()
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.ObservedGeneration).To(Equal(zone.Generation))

		By("not rewriting the copy when nothing relevant changed")
		rv := cp.ResourceVersion
		Expect(run(h)).To(Succeed())
		cp, err = getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(cp.ResourceVersion).To(Equal(rv))

		By("rotating the copy for all consumers when the source changes")
		pemB := newTestCAPEM("b")
		putSource(source, pemB, true)
		Expect(run(h)).To(Succeed())
		cp, err = getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(cp.Data["ca.crt"]).To(Equal(canonical(pemB)))
		Expect(getIDP(zone).Spec.TLS.Ref()).To(Equal(want))
	})

	It("blocks provider creation for an unlabeled first source without creating a copy", func() {
		putSource(source, newTestCAPEM("a"), false)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		create()
		err := run(newTrustHandler())
		Expect(isBlocked(err)).To(BeTrue(), "%v", err)
		_, err = getCopy(zone)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		idp := &identityapi.IdentityProvider{}
		err = k8sClient.Get(ctx, client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForIdentityProvider(zone, "primary")}, idp)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		Expect(trustCondition().Reason).To(Equal(ReasonTrustSourceInvalid))
		Expect(trustCondition().Status).To(Equal(metav1.ConditionFalse))

		By("recovering once the label is added")
		putSource(source, newTestCAPEM("a"), true)
		Expect(run(newTrustHandler())).To(Succeed())
		Expect(getIDP(zone).Spec.TLS.Ref()).NotTo(BeNil())
		Expect(trustCondition().Status).To(Equal(metav1.ConditionTrue))
	})

	It("keeps the published provider and copy unchanged while the source is invalid and publishes after repair", func() {
		pemA := newTestCAPEM("a")
		putSource(source, pemA, true)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		create()
		h := newTrustHandler()
		provisionWithTrust(h)
		oldIDP := getIDP(zone)

		putSource(source, "garbage", true)
		err := run(h)
		Expect(isBlocked(err)).To(BeTrue(), "%v", err)
		Expect(trustCondition().Reason).To(Equal(ReasonTrustSourceInvalid))
		Expect(getIDP(zone).Spec).To(Equal(oldIDP.Spec))

		deleteSource(source)
		Expect(run(h)).To(HaveOccurred())
		Expect(trustCondition().Reason).To(Equal(ReasonTrustSourceInvalid))
		cp, err := getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(cp.Data["ca.crt"]).To(Equal(canonical(pemA)))

		By("publishing again once the source is repaired")
		pemB := newTestCAPEM("b")
		putSource(source, pemB, true)
		Expect(run(h)).To(Succeed())
		Expect(trustCondition().Status).To(Equal(metav1.ConditionTrue))
		cp, err = getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(cp.Data["ca.crt"]).To(Equal(canonical(pemB)))
		Expect(getIDP(zone).Spec.TLS.Ref()).To(Equal(oldIDP.Spec.TLS.Ref()))
	})

	It("validates all sources before writing any copy and publishes nothing if one is invalid", func() {
		other := source + "-gw"
		DeferCleanup(func() { deleteSource(other) })
		putSource(source, newTestCAPEM("a"), true)
		putSource(other, newTestCAPEM("gw"), false)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		zone.Spec.Gateways[0].TLS = tlsRef(other)
		create()
		err := run(newTrustHandler())
		Expect(isBlocked(err)).To(BeTrue(), "%v", err)
		Expect(trustCondition().Reason).To(Equal(ReasonTrustSourceInvalid))
		_, err = getCopy(zone)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "no copy is written while another source is invalid")
		err = k8sClient.Get(ctx, client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForIdentityProvider(zone, "primary")}, &identityapi.IdentityProvider{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("leaves the IDP and its dependents unchanged when a changed target's source is missing", func() {
		putSource(source, newTestCAPEM("a"), true)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		create()
		h := newTrustHandler()
		provisionWithTrust(h)
		oldIDP := getIDP(zone)
		oldGW := getGateway(zone)
		oldPresets := zone.Status.Presets

		deleteSource(source)
		newURL := "https://moved-iris.de/auth/admin/realms"
		zone.Spec.IdentityProviders[0].Admin.Url = &newURL
		zone.Spec.IdentityProviders[0].IssuerHostname = "moved-iris.de"
		zone.Spec.IdentityProviders[0].TokenUrl = ""
		err := run(h)
		Expect(isBlocked(err)).To(BeTrue(), "%v", err)

		idp := getIDP(zone)
		Expect(idp.Spec.AdminUrl).To(Equal(oldIDP.Spec.AdminUrl))
		Expect(idp.Annotations[AnnotationDiscoveryIssuer]).To(Equal("https://test-iris.de/"))
		Expect(getGateway(zone).Spec.Admin.IssuerUrl).To(Equal(oldGW.Spec.Admin.IssuerUrl))
		Expect(zone.Status.Presets).To(Equal(oldPresets))
	})

	It("publishes no Gateway change while a requested source is invalid, and all after repair", func() {
		create()
		h := newTrustHandler()
		provisionWithTrust(h)
		oldURL := getGateway(zone).Spec.Admin.Url

		By("adding a gateway referencing an invalid new source")
		putSource(source, newTestCAPEM("a"), false)
		zone.Spec.Gateways = append(zone.Spec.Gateways, adminv1.GatewayConfig{
			Name: "second", TLS: tlsRef(source),
			Admin: adminv1.GatewayAdminConfig{IdentityProviderRef: "primary", Url: "https://second.example.com/admin-api"},
		})
		zone.Spec.Presets = append(zone.Spec.Presets, adminv1.Preset{
			Name: "ai", Type: adminv1.GatewayTypeAI, Default: true, GatewayRef: "second", IdentityProviderRef: "primary",
			Urls: []adminv1.UrlConfig{{Hostname: "second.example.com", BasePath: "/"}},
		})
		zone.Spec.Gateways[0].Admin.Url = "https://test-stargate.de/new-admin-api"
		err := run(h)
		Expect(isBlocked(err)).To(BeTrue(), "%v", err)
		Expect(getGateway(zone).Spec.Admin.Url).To(Equal(oldURL))
		secondKey := client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForGateway(zone, "second")}
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, secondKey, &gatewayapi.Gateway{}))).To(BeTrue())

		By("publishing both gateways once the source is repaired")
		putSource(source, newTestCAPEM("a"), true)
		Expect(run(h)).To(Succeed())
		Expect(getGateway(zone).Spec.Admin.Url).To(Equal("https://test-stargate.de/new-admin-api"))
		Expect(k8sClient.Get(ctx, secondKey, &gatewayapi.Gateway{})).To(Succeed())
	})

	It("publishes the union of all sources to every provider, including those without a source", func() {
		other := source + "-gw"
		DeferCleanup(func() { deleteSource(other) })
		pemA, pemB := newTestCAPEM("a"), newTestCAPEM("b")
		putSource(source, pemA, true)
		putSource(other, pemB+pemA, true)
		zone.Spec.Gateways[0].TLS = tlsRef(other)
		create()
		h := newTrustHandler()
		provisionWithTrust(h)

		cp, err := getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(cp.Data).To(Equal(map[string]string{"ca.crt": canonical(pemB + pemA)}))
		want := &types.CABundleConfigMapRef{Name: cp.Name, Key: providerca.ProviderCACopyKey}
		Expect(getIDP(zone).Spec.TLS.Ref()).To(Equal(want), "the IdentityProvider has no source of its own")
		Expect(getGateway(zone).Spec.Admin.TLS.Ref()).To(Equal(want))

		By("adding a second source with a duplicate certificate in another order")
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		rv := cp.ResourceVersion
		Expect(run(h)).To(Succeed())
		cp, err = getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(cp.ResourceVersion).To(Equal(rv), "an unchanged union is not rewritten")

		By("removing a source so that the bundle reflects the current union")
		putSource(source, newTestCAPEM("c")+pemA, true)
		zone.Spec.Gateways[0].TLS = nil
		Expect(run(h)).To(Succeed())
		cp, err = getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(cp.Data["ca.crt"]).NotTo(ContainSubstring(strings.TrimSpace(canonical(pemB))))
		Expect(getGateway(zone).Spec.Admin.TLS.Ref()).To(Equal(want))

		By("removing all sources: system trust is published and the bundle is retained")
		zone.Spec.IdentityProviders[0].TLS = nil
		Expect(run(h)).To(Succeed())
		Expect(getIDP(zone).Spec.TLS).To(BeNil())
		Expect(getGateway(zone).Spec.Admin.TLS).To(BeNil())
		retained, err := getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(retained.Data).To(Equal(cp.Data))
	})

	It("requires https for every provider endpoint once the Zone has custom trust", func() {
		putSource(source, newTestCAPEM("a"), true)
		zone.Spec.Gateways[0].TLS = tlsRef(source)
		plain := "http://test-iris.de/auth/admin/realms"
		zone.Spec.IdentityProviders[0].Admin.Url = &plain
		create()
		err := run(newTrustHandler())
		Expect(isBlocked(err)).To(BeTrue(), "%v", err)
		Expect(err.Error()).To(ContainSubstring("must use https"))
		_, err = getCopy(zone)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "no bundle is written for a rejected configuration")
		err = k8sClient.Get(ctx, client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForIdentityProvider(zone, "primary")}, &identityapi.IdentityProvider{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("rejects a non-https Gateway admin URL before writing the bundle", func() {
		putSource(source, newTestCAPEM("a"), true)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		zone.Spec.Gateways[0].Admin.Url = "http://test-stargate.de/admin-api"
		create()
		err := run(newTrustHandler())
		Expect(isBlocked(err)).To(BeTrue(), "%v", err)
		Expect(err.Error()).To(ContainSubstring("must use https"))
		_, err = getCopy(zone)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("writes nothing when the union exceeds the certificate limit", func() {
		other := source + "-gw"
		DeferCleanup(func() { deleteSource(other) })
		half := func(prefix string) string {
			var b strings.Builder
			for i := range serverclient.MaxProviderCABundleCerts/2 + 1 {
				b.WriteString(newTestCAPEM(fmt.Sprintf("%s-%d", prefix, i)))
			}
			return b.String()
		}
		putSource(source, half("idp"), true)
		putSource(other, half("gw"), true)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		zone.Spec.Gateways[0].TLS = tlsRef(other)
		create()
		err := run(newTrustHandler())
		Expect(isBlocked(err)).To(BeTrue(), "%v", err)
		Expect(err.Error()).To(ContainSubstring("exceeds"))
		Expect(trustCondition().Reason).To(Equal(ReasonTrustSourceInvalid))
		_, err = getCopy(zone)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Get(ctx, client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForIdentityProvider(zone, "primary")}, &identityapi.IdentityProvider{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("isolates the bundles of Zones sharing an environment", func() {
		pemA, pemB := newTestCAPEM("a"), newTestCAPEM("b")
		putSource(source, pemA, true)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		create()
		h := newTrustHandler()
		Expect(run(h)).To(Succeed())

		otherSource := source + "-other"
		DeferCleanup(func() { deleteSource(otherSource) })
		putSource(otherSource, pemB, true)
		other := newTestZone(zone.Name + "-other")
		other.Spec.IdentityProviders[0].TLS = tlsRef(otherSource)
		Expect(k8sClient.Create(ctx, other)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, other))).To(Succeed()) })
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(other), other)).To(Succeed())
		Expect(h.CreateOrUpdate(newTestContext(other), other)).To(Succeed())

		mine, err := getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		theirs, err := getCopy(other)
		Expect(err).NotTo(HaveOccurred())
		Expect(mine.Name).NotTo(Equal(theirs.Name))
		Expect(mine.Data["ca.crt"]).To(Equal(canonical(pemA)))
		Expect(theirs.Data["ca.crt"]).To(Equal(canonical(pemB)))
	})

	It("publishes IDP target moves without migrating existing Realms, also under system trust", func() {
		create()
		h := newTrustHandler()
		provisionWithTrust(h)

		newURL := "https://other-iris.de/auth/admin/realms"
		zone.Spec.IdentityProviders[0].Admin.Url = &newURL
		Expect(run(h)).To(Succeed())
		Expect(getIDP(zone).Spec.AdminUrl).To(Equal(newURL))
	})

	It("refuses to adopt or overwrite a colliding copy and publishes no provider", func() {
		putSource(source, newTestCAPEM("a"), true)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		create()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: zoneNamespace(zone)}}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
		origin := zoneOrigin(zone)
		foreign := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: origin.CopyName(), Namespace: ns.Name,
				Labels: map[string]string{types.ProviderCALabelKey: "true"},
			},
			Data: map[string]string{"ca.crt": "foreign"},
		}
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())

		err := run(newTrustHandler())
		Expect(isBlocked(err)).To(BeTrue(), "%v", err)
		Expect(trustCondition().Reason).To(Equal(ReasonTrustCopyInvalid))
		cp, err := getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(cp.Data["ca.crt"]).To(Equal("foreign"))
		err = k8sClient.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: naming.ForIdentityProvider(zone, "primary")}, &identityapi.IdentityProvider{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("repairs a deleted copy from a valid source", func() {
		putSource(source, newTestCAPEM("a"), true)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		create()
		h := newTrustHandler()
		Expect(run(h)).To(Succeed())
		cp, err := getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Delete(ctx, cp)).To(Succeed())
		Expect(run(h)).To(Succeed())
		_, err = getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
	})

	It("validates the written copy without reading it back and retries on a stale cached view", func() {
		putSource(source, newTestCAPEM("a"), true)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		create()
		h := newTrustHandler()
		origin := zoneOrigin(zone)
		h.Reader = &hiddenCopyReader{Client: k8sClient, name: origin.CopyName()}
		Expect(run(h)).To(Succeed())
		cp, err := getCopy(zone)
		Expect(err).NotTo(HaveOccurred())
		Expect(getIDP(zone).Spec.TLS.Ref().Name).To(Equal(cp.Name))

		By("returning a retryable error instead of bypassing the cache while the copy is still hidden")
		err = run(h)
		var ce *ctrlerrors.CtrlError
		Expect(errors.As(err, &ce)).To(BeTrue(), "error %v", err)
		Expect(ce.IsRetryable()).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("creating provider CA copy"))
		Expect(err.Error()).To(ContainSubstring("restore its label"))

		By("converging once the reader observes the copy")
		h.Reader = k8sClient
		Expect(run(h)).To(Succeed())
	})

	It("rejects modifying a child generated for another Zone incarnation", func() {
		create()
		h := newTrustHandler()
		provisionWithTrust(h)
		idp := getIDP(zone)
		idp.Annotations[providerca.AnnotationZoneUID] = "another-uid"
		Expect(k8sClient.Update(ctx, idp)).To(Succeed())
		err := run(h)
		Expect(isBlocked(err)).To(BeTrue(), "%v", err)
		Expect(err.Error()).To(ContainSubstring("another Zone incarnation"))
	})

	It("starts no writes for a deleted or recreated Zone", func() {
		putSource(source, newTestCAPEM("a"), true)
		zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
		create()
		stale := zone.DeepCopy()
		stale.UID = k8stypes.UID("stale-uid")
		Expect(newTrustHandler().CreateOrUpdate(newTestContext(stale), stale)).To(Succeed())
		ns := &corev1.Namespace{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: zoneNamespace(zone)}, ns)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	Context("committed Realm cleanup snapshots on a latched Zone", func() {
		var discoveries *int

		withDiscovery := func(h *ZoneHandler) *ZoneHandler {
			n := 0
			discoveries = &n
			h.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				*discoveries++
				issuer := "https://" + req.URL.Host + strings.TrimSuffix(req.URL.Path, "/.well-known/openid-configuration")
				body, err := json.Marshal(map[string]string{"issuer": issuer, "token_endpoint": issuer + "/protocol/openid-connect/token"})
				if err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}, Request: req}, nil
			})}
			return h
		}

		teamRoute := func() *gatewayapi.Route {
			GinkgoHelper()
			route := &gatewayapi.Route{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForGateway(zone, "standard") + "--team-api"}, route)).To(Succeed())
			return route
		}

		provisionLatched := func(customTrust bool) *ZoneHandler {
			GinkgoHelper()
			zone.Spec.IdentityProviders[0].TokenUrl = ""
			zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{Routes: []adminv1.ManagedRouteConfig{
				{Name: "team-api", Path: "/team-api", Url: "https://team.example.com", Type: adminv1.ManagedRouteTypeTeamAPI},
			}}
			if customTrust {
				putSource(source, newTestCAPEM("a"), true)
				zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
			}
			create()
			h := withDiscovery(newTrustHandler())
			provisionWithTrust(h)
			Expect(zone.Status.TeamApiIdentityRealm).NotTo(BeNil())
			team := &identityapi.Realm{}
			Expect(k8sClient.Get(ctx, zone.Status.TeamApiIdentityRealm.K8s(), team)).To(Succeed())
			team.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Realm is ready"))
			Expect(k8sClient.Status().Update(ctx, team)).To(Succeed())
			Expect(run(h)).To(Succeed())
			if !meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady) {
				Expect(run(h)).To(Succeed())
			}
			Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue(), "%v", zone.Status.Conditions)
			return h
		}

		for _, customTrust := range []bool{false, true} {
			It(fmt.Sprintf("publishes IDP target B to the IDP, Gateway issuer and routes (custom trust: %v)", customTrust), func() {
				h := provisionLatched(customTrust)

				newURL := "https://iris-b.de/auth/admin/realms"
				zone.Spec.IdentityProviders[0].Admin.Url = &newURL
				zone.Spec.IdentityProviders[0].IssuerHostname = "iris-b.de"
				Expect(run(h)).To(Succeed())

				Expect(getIDP(zone).Spec.AdminUrl).To(Equal(newURL))
				Expect(getIDP(zone).Annotations[AnnotationDiscoveryIssuer]).To(Equal("https://iris-b.de/"))
				Expect(getGateway(zone).Spec.Admin.IssuerUrl).To(HavePrefix("https://iris-b.de/"))
				Expect(teamRoute().Spec.Security.TrustedIssuers).To(ConsistOf(HavePrefix("https://iris-b.de/")))
			})
		}

		It("rotates the bundle together with a target move", func() {
			h := provisionLatched(true)
			pemB := newTestCAPEM("b")
			putSource(source, pemB, true)
			newURL := "https://iris-b.de/auth/admin/realms"
			zone.Spec.IdentityProviders[0].Admin.Url = &newURL
			Expect(run(h)).To(Succeed())

			rotated, err := getCopy(zone)
			Expect(err).NotTo(HaveOccurred())
			Expect(rotated.Data["ca.crt"]).To(Equal(canonical(pemB)))
			Expect(getIDP(zone).Spec.AdminUrl).To(Equal(newURL))
			Expect(getIDP(zone).Spec.TLS.Ref().Name).To(Equal(rotated.Name))
		})

		It("retains copies and children on Zone deletion for historical cleanup", func() {
			h := provisionLatched(true)
			cp, err := getCopy(zone)
			Expect(err).NotTo(HaveOccurred())
			Expect(h.Delete(newTestContext(zone), zone)).To(Succeed())
			Expect(k8sClient.Delete(ctx, zone)).To(Succeed())
			deleteSource(source)

			By("allowing a manual rotation of the retained copy that preserves label and origin")
			retained, err := getCopy(zone)
			Expect(err).NotTo(HaveOccurred())
			Expect(retained.UID).To(Equal(cp.UID))
			pemC := newTestCAPEM("c")
			retained.Data["ca.crt"] = pemC
			Expect(k8sClient.Update(ctx, retained)).To(Succeed())
			origin := zoneOrigin(zone)
			bundle, err := ValidateProviderCACopy(retained, &origin)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bundle.CanonicalPEM())).To(Equal(canonical(pemC)))
			Expect(getIDP(zone).Spec.TLS.Ref().Name).To(Equal(cp.Name))
		})
	})
})

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// hiddenCopyReader simulates an informer cache that has not yet observed provider CA copies.
type hiddenCopyReader struct {
	client.Client
	name string
}

func (r *hiddenCopyReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*corev1.ConfigMap); ok && key.Name == r.name {
		return apierrors.NewNotFound(corev1.Resource("configmaps"), key.Name)
	}
	return r.Client.Get(ctx, key, obj, opts...)
}
