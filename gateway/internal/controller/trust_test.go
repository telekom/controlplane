// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/providerca"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	gatewayhandler "github.com/telekom/controlplane/gateway/internal/handler/gateway"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func selfSignedCAPEM() string {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "provider-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// nudge triggers a reconciliation through a label change, standing in for the periodic requeue:
// ConfigMap changes do not enqueue Gateways, Routes or Consumers.
func nudge(obj client.Object) {
	GinkgoHelper()
	Eventually(func() error {
		if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
			return err
		}
		labels := obj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels["test.cp.ei.telekom.de/nudge"] = strconv.FormatInt(time.Now().UnixNano(), 10)
		obj.SetLabels(labels)
		return k8sClient.Update(context.Background(), obj)
	}, timeout, interval).Should(Succeed())
}

var _ = Describe("Provider trust on reconciliation", Ordered, func() {
	const (
		namespace = "trust-ns"
		gwName    = "trust-gw"
		cmName    = "trust-ca"
	)
	ctx := context.Background()
	gwKey := ktypes.NamespacedName{Namespace: namespace, Name: gwName}
	var (
		cm       *corev1.ConfigMap
		route    *gatewayv1.Route
		consumer *gatewayv1.Consumer
	)

	trustStatus := func(obj ctypes.Object) func(g Gomega) metav1.ConditionStatus {
		return func(g Gomega) metav1.ConditionStatus {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
			c := meta.FindStatusCondition(obj.GetConditions(), gatewayhandler.ConditionTypeTrustReady)
			if c == nil {
				return ""
			}
			return c.Status
		}
	}

	setLabel := func(labeled bool) {
		expireTrust()
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)).To(Succeed())
		if labeled {
			cm.Labels = map[string]string{ctypes.ProviderCALabelKey: ctypes.ProviderCALabelValue}
		} else {
			cm.Labels = map[string]string{ctypes.ProviderCALabelKey: "false"}
		}
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())
	}

	BeforeAll(func() {
		createNamespace(namespace)
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: cmName, Namespace: namespace,
				Labels: map[string]string{ctypes.ProviderCALabelKey: ctypes.ProviderCALabelValue},
			},
			Data: map[string]string{"ca.crt": selfSignedCAPEM()},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		gw := newGateway(gwName, namespace)
		// Custom trust applies to the admin and issuer URLs, so both must be https.
		gw.Spec.Admin.Url = "https://kong-admin:8001"
		gw.Spec.Admin.IssuerUrl = "https://issuer:8443/realms/test"
		gw.Spec.Admin.TLS = &ctypes.ProviderTLS{CABundleConfigMapRef: &ctypes.CABundleConfigMapRef{Name: cmName, Key: "ca.crt"}}
		Expect(k8sClient.Create(ctx, gw)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, gwKey, gw)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(gw.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(gw.GetConditions(), gatewayhandler.ConditionTypeTrustReady)).To(BeTrue())
		}, timeout, interval).Should(Succeed())

		route = &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "trust-route", Namespace: namespace,
				Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment},
			},
			Spec: gatewayv1.RouteSpec{
				GatewayRef: ctypes.ObjectRef{Name: gwName, Namespace: namespace},
				Type:       gatewayv1.RouteTypePrimary,
				Backend: gatewayv1.Backend{
					Upstreams: []gatewayv1.Upstream{{Scheme: "https", Hostname: "backend.example.com", Port: 443}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, route)).To(Succeed())
		consumer = &gatewayv1.Consumer{
			ObjectMeta: metav1.ObjectMeta{
				Name: "trust-consumer", Namespace: namespace,
				Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment},
			},
			Spec: gatewayv1.ConsumerSpec{
				Gateway: ctypes.ObjectRef{Name: gwName, Namespace: namespace},
				Name:    "trust-consumer",
			},
		}
		Expect(k8sClient.Create(ctx, consumer)).To(Succeed())

		Eventually(trustStatus(route), timeout, interval).Should(Equal(metav1.ConditionTrue))
		Eventually(trustStatus(consumer), timeout, interval).Should(Equal(metav1.ConditionTrue))
	})

	It("keeps unlabeled ConfigMaps out of the manager cache but readable directly", func() {
		other := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: namespace},
			Data:       map[string]string{"k": "v"},
		}
		Expect(k8sClient.Create(ctx, other)).To(Succeed())
		Consistently(func() bool {
			return apierrors.IsNotFound(managerCache.Get(ctx, client.ObjectKeyFromObject(other), &corev1.ConfigMap{}))
		}, time.Second, interval).Should(BeTrue())
		Expect(apiReader.Get(ctx, client.ObjectKeyFromObject(other), &corev1.ConfigMap{})).To(Succeed())
		Eventually(func() error {
			return managerCache.Get(ctx, client.ObjectKeyFromObject(cm), &corev1.ConfigMap{})
		}, timeout, interval).Should(Succeed())
	})

	It("observes label removal on the next reconciliation, not through ConfigMap events", func() {
		setLabel(false)
		Consistently(trustStatus(route), time.Second, interval).Should(Equal(metav1.ConditionTrue))

		nudge(route)
		nudge(consumer)
		Eventually(trustStatus(route), timeout, interval).Should(Equal(metav1.ConditionFalse))
		Eventually(trustStatus(consumer), timeout, interval).Should(Equal(metav1.ConditionFalse))
		// The label-filtered informer cache no longer holds the ConfigMap.
		Expect(meta.FindStatusCondition(route.GetConditions(), gatewayhandler.ConditionTypeTrustReady).Reason).To(Equal("ConfigMapNotFound"))
	})

	It("recovers on the next reconciliation when the label is added again", func() {
		setLabel(true)
		nudge(route)
		nudge(consumer)
		Eventually(trustStatus(route), timeout, interval).Should(Equal(metav1.ConditionTrue))
		Eventually(trustStatus(consumer), timeout, interval).Should(Equal(metav1.ConditionTrue))
	})

	It("blocks a terminating route on invalid trust and completes cleanup after repair", func() {
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)).To(Succeed())
		good := cm.Data["ca.crt"]
		cm.Data["ca.crt"] = "not a certificate"
		expireTrust()
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())
		nudge(route)
		Eventually(trustStatus(route), timeout, interval).Should(Equal(metav1.ConditionFalse))

		Expect(k8sClient.Delete(ctx, route)).To(Succeed())
		Consistently(func(g Gomega) {
			cur := &gatewayv1.Route{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), cur)).To(Succeed())
			g.Expect(cur.DeletionTimestamp).NotTo(BeNil())
			g.Expect(cur.Finalizers).NotTo(BeEmpty())
		}, time.Second, interval).Should(Succeed())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)).To(Succeed())
		cm.Data["ca.crt"] = good
		expireTrust()
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())
		nudge(route)
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), &gatewayv1.Route{}))
		}, timeout, interval).Should(BeTrue())
	})

	It("fans out Gateway spec changes to consumers", func() {
		gw := &gatewayv1.Gateway{}
		Expect(k8sClient.Get(ctx, gwKey, gw)).To(Succeed())
		gw.Spec.Admin.TLS.CABundleConfigMapRef.Name = "missing-ca"
		Expect(k8sClient.Update(ctx, gw)).To(Succeed())
		Eventually(trustStatus(consumer), timeout, interval).Should(Equal(metav1.ConditionFalse))
		Expect(meta.FindStatusCondition(consumer.GetConditions(), gatewayhandler.ConditionTypeTrustReady).Reason).
			To(Equal("ConfigMapNotFound"))
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, gwKey, gw)).To(Succeed())
			g.Expect(meta.IsStatusConditionFalse(gw.GetConditions(), gatewayhandler.ConditionTypeTrustReady)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	It("recovers when a missing ConfigMap is created with the label", func() {
		created := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "missing-ca", Namespace: namespace,
				Labels: map[string]string{ctypes.ProviderCALabelKey: ctypes.ProviderCALabelValue},
			},
			Data: map[string]string{"ca.crt": selfSignedCAPEM()},
		}
		expireTrust()
		Expect(k8sClient.Create(ctx, created)).To(Succeed())
		nudge(consumer)
		Eventually(trustStatus(consumer), timeout, interval).Should(Equal(metav1.ConditionTrue))
	})

	AfterAll(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, consumer))).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(consumer), &gatewayv1.Consumer{}))
		}, timeout, interval).Should(BeTrue())
		gw := &gatewayv1.Gateway{}
		gw.Name, gw.Namespace = gwName, namespace
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, gw))).To(Succeed())
	})
})

var _ = Describe("Provider CA copy provenance", Ordered, func() {
	const (
		namespace = "trust-prov-ns"
		gwName    = "prov-gw"
	)
	ctx := context.Background()
	origin := &providerca.ProviderCAOrigin{
		ZoneUID: "zone-uid-1", ZoneNamespace: "zone-ns", ZoneName: "zone-a",
	}
	var (
		copyCM   *corev1.ConfigMap
		pemData  string
		route    *gatewayv1.Route
		consumer *gatewayv1.Consumer
	)

	trust := func(obj ctypes.Object) func(g Gomega) metav1.ConditionStatus {
		return func(g Gomega) metav1.ConditionStatus {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
			c := meta.FindStatusCondition(obj.GetConditions(), gatewayhandler.ConditionTypeTrustReady)
			if c == nil {
				return ""
			}
			return c.Status
		}
	}

	// setHash changes only the origin hash annotation; data and labels stay unchanged.
	setHash := func(hash string) {
		expireTrust()
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(copyCM), copyCM)).To(Succeed())
		copyCM.Annotations[providerca.AnnotationProviderCAOriginHash] = hash
		Expect(k8sClient.Update(ctx, copyCM)).To(Succeed())
		Expect(copyCM.Data[providerca.ProviderCACopyKey]).To(Equal(pemData))
	}

	newDependents := func(suffix string) {
		route = &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "prov-route-" + suffix, Namespace: namespace,
				Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment},
			},
			Spec: gatewayv1.RouteSpec{
				GatewayRef: ctypes.ObjectRef{Name: gwName, Namespace: namespace},
				Type:       gatewayv1.RouteTypePrimary,
				Backend: gatewayv1.Backend{
					Upstreams: []gatewayv1.Upstream{{Scheme: "https", Hostname: "backend.example.com", Port: 443}},
				},
			},
		}
		consumer = &gatewayv1.Consumer{
			ObjectMeta: metav1.ObjectMeta{
				Name: "prov-consumer-" + suffix, Namespace: namespace,
				Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment},
			},
			Spec: gatewayv1.ConsumerSpec{
				Gateway: ctypes.ObjectRef{Name: gwName, Namespace: namespace},
				Name:    "prov-consumer-" + suffix,
			},
		}
		Expect(k8sClient.Create(ctx, route)).To(Succeed())
		Expect(k8sClient.Create(ctx, consumer)).To(Succeed())
		Eventually(trust(route), timeout, interval).Should(Equal(metav1.ConditionTrue))
		Eventually(trust(consumer), timeout, interval).Should(Equal(metav1.ConditionTrue))
	}

	BeforeAll(func() {
		createNamespace(namespace)
		pemData = selfSignedCAPEM()
		copyCM = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: origin.CopyName(), Namespace: namespace, Annotations: origin.Annotations(),
				Labels: map[string]string{
					ctypes.ProviderCALabelKey:    ctypes.ProviderCALabelValue,
					config.EnvironmentLabelKey:   testEnvironment,
					config.BuildLabelKey("zone"): origin.ZoneName,
				},
			},
			Data: map[string]string{providerca.ProviderCACopyKey: pemData},
		}
		Expect(k8sClient.Create(ctx, copyCM)).To(Succeed())

		gw := newGateway(gwName, namespace)
		gw.Spec.Admin.Url = "https://kong-admin:8001"
		gw.Spec.Admin.IssuerUrl = "https://issuer:8443/realms/test"
		caRef := &ctypes.CABundleConfigMapRef{Name: origin.CopyName(), Key: providerca.ProviderCACopyKey}
		gw.Spec.Admin.TLS = &ctypes.ProviderTLS{CABundleConfigMapRef: caRef}
		Expect(k8sClient.Create(ctx, gw)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), gw)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(gw.GetConditions(), gatewayhandler.ConditionTypeTrustReady)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	It("blocks and recovers active dependents on provenance-only corruption and repair", func() {
		newDependents("active")
		gwGen := func() int64 {
			gw := &gatewayv1.Gateway{}
			Expect(k8sClient.Get(ctx, ktypes.NamespacedName{Namespace: namespace, Name: gwName}, gw)).To(Succeed())
			return gw.Generation
		}
		gen := gwGen()

		setHash("corrupted")
		nudge(route)
		nudge(consumer)
		Eventually(trust(route), timeout, interval).Should(Equal(metav1.ConditionFalse))
		Eventually(trust(consumer), timeout, interval).Should(Equal(metav1.ConditionFalse))
		Expect(meta.FindStatusCondition(route.GetConditions(), gatewayhandler.ConditionTypeTrustReady).Reason).
			To(Equal("InvalidProvenance"))

		setHash(origin.Hash())
		nudge(route)
		nudge(consumer)
		Eventually(trust(route), timeout, interval).Should(Equal(metav1.ConditionTrue))
		Eventually(trust(consumer), timeout, interval).Should(Equal(metav1.ConditionTrue))
		Expect(gwGen()).To(Equal(gen), "recovery must not rely on Gateway spec changes")
	})
})
