// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	"github.com/telekom/controlplane/common/pkg/cabundle"
	"github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// newTestCertPEM returns a self-signed PEM certificate, a CA if isCA is set.
func newTestCertPEM(cn string, isCA bool) string {
	GinkgoHelper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	Expect(err).NotTo(HaveOccurred())
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}
	if isCA {
		tmpl.KeyUsage = x509.KeyUsageCertSign
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func tlsRef(name, key string) *types.TLSConfig {
	return &types.TLSConfig{ConfigMapRef: types.ConfigMapKeyRef{Name: name, Key: key}}
}

func upsertConfigMap(namespace, name string, labels, data map[string]string) {
	GinkgoHelper()
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
	if apierrors.IsNotFound(err) {
		cm.Labels = labels
		cm.Data = data
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		return
	}
	Expect(err).NotTo(HaveOccurred())
	cm.Labels = labels
	cm.Data = data
	Expect(k8sClient.Update(ctx, cm)).To(Succeed())
}

func managedBundleKey(zone *adminv1.Zone) client.ObjectKey {
	return client.ObjectKey{Namespace: zoneNamespace(zone), Name: cabundle.ManagedBundleName}
}

func getManagedBundle(zone *adminv1.Zone) *corev1.ConfigMap {
	GinkgoHelper()
	cm := &corev1.ConfigMap{}
	Expect(k8sClient.Get(ctx, managedBundleKey(zone), cm)).To(Succeed())
	return cm
}

func expectNoManagedBundle(zone *adminv1.Zone) {
	GinkgoHelper()
	err := k8sClient.Get(ctx, managedBundleKey(zone), &corev1.ConfigMap{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected no managed bundle, got %v", err)
}

func getZoneIdentityProvider(zone *adminv1.Zone) *identityapi.IdentityProvider {
	GinkgoHelper()
	idp := &identityapi.IdentityProvider{}
	key := client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForIdentityProvider(zone, "primary")}
	Expect(k8sClient.Get(ctx, key, idp)).To(Succeed())
	return idp
}

func expectBlocked(err error) {
	GinkgoHelper()
	var blocked ctrlerrors.BlockedError
	Expect(errors.As(err, &blocked)).To(BeTrue(), "expected a blocked error, got %v", err)
	Expect(blocked.IsBlocked()).To(BeTrue(), "expected a blocked error, got %v", err)
}

func expectRetryable(err error) {
	GinkgoHelper()
	var retryable ctrlerrors.RetryableError
	Expect(errors.As(err, &retryable)).To(BeTrue(), "expected a retryable error, got %v", err)
	Expect(retryable.IsRetryable()).To(BeTrue(), "expected a retryable error, got %v", err)
}

func zoneLabels(zone *adminv1.Zone) map[string]string {
	return map[string]string{
		config.EnvironmentLabelKey:          zone.Labels[config.EnvironmentLabelKey],
		config.BuildLabelKey(zoneLabelName): zone.Name,
		config.DomainLabelKey:               domainName,
	}
}

// configMapClient fails ConfigMap reads with a transient error.
type configMapClient struct {
	client.Client
	getErr error
}

func (c *configMapClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*corev1.ConfigMap); ok {
		return c.getErr
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

var _ = Describe("Zone CA bundle", func() {
	var (
		h          *ZoneHandler
		zone       *adminv1.Zone
		caGateway  string
		caIDP      string
		gatewaySrc string
		idpSrc     string
	)

	BeforeEach(func() {
		h = newTestHandler()
		caGateway = newTestCertPEM("gateway-ca", true)
		caIDP = newTestCertPEM("idp-ca", true)

		zone = uniqueZone("ca")
		withAIGateway(zone)
		gatewaySrc = zone.Name + "-gateway-ca"
		idpSrc = zone.Name + "-idp-ca"
		upsertConfigMap(testNamespace, gatewaySrc, nil, map[string]string{"ca.pem": caGateway})
		upsertConfigMap(testNamespace, idpSrc, nil, map[string]string{"bundle.pem": caIDP})
		zone.Spec.Gateways[0].TLS = tlsRef(gatewaySrc, "ca.pem")
		zone.Spec.IdentityProviders[0].TLS = tlsRef(idpSrc, "bundle.pem")
	})

	expectedUnion := func(sets ...string) string {
		GinkgoHelper()
		pems := make([][]byte, 0, len(sets))
		for _, s := range sets {
			pems = append(pems, []byte(s))
		}
		union, err := cabundle.Canonicalize(pems...)
		Expect(err).NotTo(HaveOccurred())
		return string(union)
	}

	It("publishes the union of all sources and references it from every child", func() {
		provision(h, zone)

		cm := getManagedBundle(zone)
		Expect(cm.Labels).To(Equal(zoneLabels(zone)))
		Expect(cm.Data).To(Equal(map[string]string{cabundle.ManagedBundleKey: expectedUnion(caGateway, caIDP)}))

		managed := tlsRef(cabundle.ManagedBundleName, cabundle.ManagedBundleKey)
		Expect(getZoneIdentityProvider(zone).Spec.TLS).To(Equal(managed))
		gateways := gatewaysOf(zone)
		Expect(gateways).To(HaveLen(2))
		for i := range gateways {
			Expect(gateways[i].Spec.TLS).To(Equal(managed))
		}
	})

	DescribeTable("keeps the last valid bundle and children when a source becomes invalid",
		func(invalid func() string) {
			provision(h, zone)
			before := getManagedBundle(zone)
			gatewayBefore := gatewaysOf(zone)
			idpBefore := getZoneIdentityProvider(zone).Spec.TLS

			upsertConfigMap(testNamespace, gatewaySrc, nil, map[string]string{"ca.pem": invalid()})
			zone.Spec.Gateways[0].Admin.Url = "https://changed.example.com/admin-api"

			expectBlocked(h.CreateOrUpdate(newTestContext(zone), zone))

			after := getManagedBundle(zone)
			Expect(after.ResourceVersion).To(Equal(before.ResourceVersion))
			Expect(after.Data).To(Equal(before.Data))
			Expect(gatewaysOf(zone)).To(Equal(gatewayBefore))
			Expect(getZoneIdentityProvider(zone).Spec.TLS).To(Equal(idpBefore))
		},
		Entry("leaf certificate", func() string { return newTestCertPEM("leaf", false) }),
		Entry("malformed block followed by a valid CA", func() string {
			return "-----BEGIN CERTIFICATE-----\n!!!!\n-----END CERTIFICATE-----\n" + newTestCertPEM("later", true)
		}),
		Entry("unmatched BEGIN followed by a valid CA", func() string {
			return "-----BEGIN CERTIFICATE-----\nAAAA\n" + newTestCertPEM("later", true)
		}),
	)

	It("blocks on a missing source ConfigMap or key without creating children", func() {
		zone.Spec.IdentityProviders[0].TLS = tlsRef("does-not-exist", "ca.pem")
		expectBlocked(h.CreateOrUpdate(newTestContext(zone), zone))

		zone.Spec.IdentityProviders[0].TLS = tlsRef(idpSrc, "missing.pem")
		expectBlocked(h.CreateOrUpdate(newTestContext(zone), zone))

		expectNoManagedBundle(zone)
		Expect(gatewaysOf(zone)).To(BeEmpty())
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForIdentityProvider(zone, "primary")}, &identityapi.IdentityProvider{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("retries transient source read errors", func() {
		h.Client = &configMapClient{Client: k8sClient, getErr: errors.New("connection refused")}
		expectRetryable(h.CreateOrUpdate(newTestContext(zone), zone))
		expectNoManagedBundle(zone)
	})

	It("retries when the cache has not observed the managed bundle yet", func() {
		provision(h, zone)
		before := getManagedBundle(zone)
		extra := newTestCertPEM("extra", true)
		upsertConfigMap(testNamespace, gatewaySrc, nil, map[string]string{"ca.pem": caGateway + extra})

		h.Client = &managedNotFoundClient{Client: k8sClient}
		expectRetryable(h.CreateOrUpdate(newTestContext(zone), zone))
		Expect(getManagedBundle(zone).Data).To(Equal(before.Data))

		h.Client = k8sClient
		Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(getManagedBundle(zone).Data).To(Equal(map[string]string{cabundle.ManagedBundleKey: expectedUnion(caGateway, extra, caIDP)}))
	})

	It("does not overwrite an unmanaged ConfigMap with the bundle name", func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: zoneNamespace(zone)}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		foreign := map[string]string{"other": "value"}
		upsertConfigMap(ns.Name, cabundle.ManagedBundleName, map[string]string{config.EnvironmentLabelKey: "other"}, foreign)

		expectBlocked(h.CreateOrUpdate(newTestContext(zone), zone))
		Expect(getManagedBundle(zone).Data).To(Equal(foreign))
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: naming.ForIdentityProvider(zone, "primary")}, &identityapi.IdentityProvider{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("adopts a retained bundle managed for the same environment and zone", func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: zoneNamespace(zone)}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		upsertConfigMap(ns.Name, cabundle.ManagedBundleName, zoneLabels(zone), map[string]string{cabundle.ManagedBundleKey: "stale", "extra": "x"})

		provision(h, zone)
		Expect(getManagedBundle(zone).Data).To(Equal(map[string]string{cabundle.ManagedBundleKey: expectedUnion(caGateway, caIDP)}))
	})

	It("retains the bundle on zone deletion", func() {
		provision(h, zone)
		Expect(h.Delete(newTestContext(zone), zone)).To(Succeed())
		getManagedBundle(zone)
	})

	It("retires the bundle after all references are gone", func() {
		provision(h, zone)
		ns := zoneNamespace(zone)
		managed := tlsRef(cabundle.ManagedBundleName, cabundle.ManagedBundleKey)

		By("adding a retained managed gateway that is no longer in the spec")
		retained := &gatewayapi.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "retained", Namespace: ns, Labels: zoneLabels(zone)},
			Spec:       gatewayapi.GatewaySpec{Admin: gatewayapi.AdminConfig{Url: "https://retained.example.com"}, TLS: managed},
		}
		Expect(k8sClient.Create(ctx, retained)).To(Succeed())

		By("adding an unmanaged gateway and a realm referencing the bundle")
		unmanaged := &gatewayapi.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "unmanaged", Namespace: ns},
			Spec:       gatewayapi.GatewaySpec{Admin: gatewayapi.AdminConfig{Url: "https://unmanaged.example.com"}, TLS: managed},
		}
		Expect(k8sClient.Create(ctx, unmanaged)).To(Succeed())
		realm := &identityapi.Realm{
			ObjectMeta: metav1.ObjectMeta{Name: "consumer", Namespace: ns},
			Spec:       identityapi.RealmSpec{IdentityProvider: &types.ObjectRef{Name: "idp", Namespace: ns}},
		}
		Expect(k8sClient.Create(ctx, realm)).To(Succeed())
		realm.Status.Connection = &identityapi.RealmConnection{
			IdentityProvider: types.ObjectRef{Name: "idp", Namespace: ns, UID: "uid"},
			TLS:              &identityapi.RealmConnectionTLS{Namespace: ns, ConfigMapRef: managedBundleRef()},
		}
		Expect(k8sClient.Status().Update(ctx, realm)).To(Succeed())

		By("removing all CA sources")
		zone.Spec.Gateways[0].TLS = nil
		zone.Spec.IdentityProviders[0].TLS = nil
		Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())

		Expect(getZoneIdentityProvider(zone).Spec.TLS).To(BeNil())
		for _, gw := range gatewaysOf(zone) {
			if gw.Name == unmanaged.Name {
				Expect(gw.Spec.TLS).To(Equal(managed))
				continue
			}
			Expect(gw.Spec.TLS).To(BeNil(), "gateway %s", gw.Name)
		}
		getManagedBundle(zone)

		By("keeping the bundle while the realm references it")
		Expect(k8sClient.Delete(ctx, unmanaged)).To(Succeed())
		Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		getManagedBundle(zone)

		By("deleting the bundle once the realm no longer references it")
		realm.Status.Connection.TLS = nil
		Expect(k8sClient.Status().Update(ctx, realm)).To(Succeed())
		Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		expectNoManagedBundle(zone)
	})

	It("does not retire an unmanaged ConfigMap with the bundle name", func() {
		zone.Spec.Gateways[0].TLS = nil
		zone.Spec.IdentityProviders[0].TLS = nil
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: zoneNamespace(zone)}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		upsertConfigMap(ns.Name, cabundle.ManagedBundleName, nil, map[string]string{"k": "v"})

		provision(h, zone)
		Expect(getManagedBundle(zone).Data).To(Equal(map[string]string{"k": "v"}))
	})

	It("panics without a Kubernetes client", func() {
		Expect(func() { _ = (&ZoneHandler{}).CreateOrUpdate(newTestContext(zone), zone) }).To(PanicWith("ZoneHandler.Client must be set"))
	})
})

// managedNotFoundClient reports the managed bundle as missing, like a lagging cache.
type managedNotFoundClient struct {
	client.Client
}

func (c *managedNotFoundClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*corev1.ConfigMap); ok && key.Name == cabundle.ManagedBundleName {
		return apierrors.NewNotFound(corev1.Resource("configmaps"), key.Name)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}
