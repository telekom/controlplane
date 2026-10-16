// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/cabundle"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newCAPEM(cn string) string {
	GinkgoHelper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func canonical(sets ...string) string {
	GinkgoHelper()
	pems := make([][]byte, 0, len(sets))
	for _, s := range sets {
		pems = append(pems, []byte(s))
	}
	out, err := cabundle.Canonicalize(pems...)
	Expect(err).NotTo(HaveOccurred())
	return string(out)
}

func ensureEnvironment() {
	GinkgoHelper()
	env := &adminv1.Environment{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: testEnvironment, Namespace: testEnvironment}, env)
	if !apierrors.IsNotFound(err) {
		Expect(err).NotTo(HaveOccurred())
		return
	}
	env = &adminv1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testEnvironment,
			Namespace: testEnvironment,
			Labels:    map[string]string{config.EnvironmentLabelKey: testEnvironment},
		},
		Spec: adminv1.EnvironmentSpec{RealmName: testEnvironment},
	}
	Expect(k8sClient.Create(ctx, env)).To(Succeed())
}

const caSourceName = "zone-ca-source"

func setSourceCA(value string) {
	GinkgoHelper()
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: caSourceName, Namespace: testEnvironment}}
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
	if apierrors.IsNotFound(err) {
		cm.Data = map[string]string{"ca.pem": value}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		return
	}
	Expect(err).NotTo(HaveOccurred())
	cm.Data = map[string]string{"ca.pem": value}
	Expect(k8sClient.Update(ctx, cm)).To(Succeed())
}

func bundleKey(zoneName string) client.ObjectKey {
	return client.ObjectKey{Namespace: testEnvironment + "--" + zoneName, Name: cabundle.ManagedBundleName}
}

func bundleData(g Gomega, zoneName string) string {
	cm := &corev1.ConfigMap{}
	g.Expect(k8sClient.Get(ctx, bundleKey(zoneName), cm)).To(Succeed())
	return cm.Data[cabundle.ManagedBundleKey]
}

var _ = Describe("Zone CA bundle propagation", func() {
	It("indexes the referenced ConfigMap names once per zone", func() {
		zone := newZone("index")
		ref := &types.TLSConfig{ConfigMapRef: types.ConfigMapKeyRef{Name: "shared", Key: "a"}}
		zone.Spec.Gateways[0].TLS = ref
		zone.Spec.IdentityProviders[0].TLS = &types.TLSConfig{ConfigMapRef: types.ConfigMapKeyRef{Name: "shared", Key: "b"}}
		Expect(indexCAConfigMapNames(zone)).To(Equal([]string{"shared"}))
		Expect(indexCAConfigMapNames(&corev1.ConfigMap{})).To(BeNil())
	})

	It("propagates source changes, repairs the managed bundle and blocks on invalid sources", func() {
		ensureEnvironment()
		caA := newCAPEM("a")
		setSourceCA(caA)

		ref := &types.TLSConfig{ConfigMapRef: types.ConfigMapKeyRef{Name: caSourceName, Key: "ca.pem"}}
		zones := []*adminv1.Zone{newZone("ca-one"), newZone("ca-two")}
		zones[0].Spec.Gateways[0].TLS = ref
		zones[1].Spec.IdentityProviders[0].TLS = ref
		for _, zone := range zones {
			Expect(k8sClient.Create(ctx, zone)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed())
			})
		}

		By("publishing the bundle and referencing it from the gateways")
		Eventually(func(g Gomega) {
			for _, zone := range zones {
				g.Expect(bundleData(g, zone.Name)).To(Equal(canonical(caA)))
				gateways := &gatewayv1.GatewayList{}
				g.Expect(k8sClient.List(ctx, gateways, client.InNamespace(bundleKey(zone.Name).Namespace))).To(Succeed())
				g.Expect(gateways.Items).NotTo(BeEmpty())
				for i := range gateways.Items {
					g.Expect(gateways.Items[i].Spec.TLS).NotTo(BeNil())
					g.Expect(gateways.Items[i].Spec.TLS.ConfigMapRef.Name).To(Equal(cabundle.ManagedBundleName))
				}
			}
		}, timeout, interval).Should(Succeed())

		By("updating every referencing zone when the shared source changes")
		caB := newCAPEM("b")
		setSourceCA(caA + caB)
		Eventually(func(g Gomega) {
			for _, zone := range zones {
				g.Expect(bundleData(g, zone.Name)).To(Equal(canonical(caA, caB)))
			}
		}, timeout, interval).Should(Succeed())

		By("repairing an edited managed bundle")
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, bundleKey(zones[0].Name), cm)).To(Succeed())
		cm.Data[cabundle.ManagedBundleKey] = "tampered"
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(bundleData(g, zones[0].Name)).To(Equal(canonical(caA, caB)))
		}, timeout, interval).Should(Succeed())

		By("recreating a deleted managed bundle")
		Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(bundleData(g, zones[0].Name)).To(Equal(canonical(caA, caB)))
		}, timeout, interval).Should(Succeed())

		By("blocking on an invalid source while keeping the last valid bundle")
		setSourceCA("junk")
		Eventually(func(g Gomega) {
			got := &adminv1.Zone{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zones[1]), got)).To(Succeed())
			ready := meta.FindStatusCondition(got.Status.Conditions, condition.ConditionTypeReady)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(ready.Message).To(ContainSubstring("invalid CA configuration"))
		}, timeout, interval).Should(Succeed())
		Consistently(func(g Gomega) {
			g.Expect(bundleData(g, zones[1].Name)).To(Equal(canonical(caA, caB)))
		}, time.Second, interval).Should(Succeed())

		By("recovering once the source is valid again")
		setSourceCA(caB)
		Eventually(func(g Gomega) {
			g.Expect(bundleData(g, zones[1].Name)).To(Equal(canonical(caB)))
		}, timeout, interval).Should(Succeed())
	})
})
