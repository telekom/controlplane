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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	zone_handler "github.com/telekom/controlplane/admin/internal/handler/zone"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/providerca"
	"github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func testCAPEM(cn string) string {
	GinkgoHelper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

const watchedSource = "watched-provider-ca"

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
			Name: testEnvironment, Namespace: testEnvironment,
			Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment},
		},
		Spec: adminv1.EnvironmentSpec{RealmName: testEnvironment},
	}
	Expect(k8sClient.Create(ctx, env)).To(Succeed())
}

func copyKey(zone *adminv1.Zone) client.ObjectKey {
	return client.ObjectKey{Namespace: zone.Status.Namespace, Name: zoneCopyName(zone)}
}

var _ = Describe("Zone provider CA watches", func() {
	Context("index and predicate", func() {
		It("indexes namespace-qualified source references once", func() {
			zone := newZone("idx")
			zone.Spec.IdentityProviders[0].TLS = &types.ProviderTLS{CABundleConfigMapRef: &types.CABundleConfigMapRef{Name: "s", Key: "a"}}
			zone.Spec.Gateways[0].TLS = &types.ProviderTLS{CABundleConfigMapRef: &types.CABundleConfigMapRef{Name: "s", Key: "b"}}
			Expect(indexZoneProviderCASources(zone)).To(Equal([]string{testEnvironment + "/s"}))
			Expect(indexZoneProviderCASources(newZone("none"))).To(BeEmpty())
		})

		It("passes only semantically relevant updates", func() {
			base := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c", ResourceVersion: "1",
					Labels: map[string]string{types.ProviderCALabelKey: "true"},
				},
				Data: map[string]string{"k": "v"},
			}
			update := func(mut func(*corev1.ConfigMap)) bool {
				n := base.DeepCopy()
				n.ResourceVersion = "2"
				mut(n)
				return ProviderCAConfigMapPredicate.Update(event.UpdateEvent{ObjectOld: base, ObjectNew: n})
			}
			Expect(update(func(*corev1.ConfigMap) {})).To(BeFalse())
			Expect(update(func(c *corev1.ConfigMap) { c.Labels["unrelated"] = "x" })).To(BeFalse())
			Expect(update(func(c *corev1.ConfigMap) { c.Data["k"] = "w" })).To(BeTrue())
			Expect(update(func(c *corev1.ConfigMap) { c.Labels[types.ProviderCALabelKey] = "false" })).To(BeTrue())
			Expect(update(func(c *corev1.ConfigMap) { c.Annotations = map[string]string{"a": "b"} })).To(BeFalse())
			Expect(update(func(c *corev1.ConfigMap) {
				c.Annotations = map[string]string{providerca.AnnotationZoneUID: "u"}
			})).To(BeTrue())
			Expect(ProviderCAConfigMapPredicate.Delete(event.DeleteEvent{Object: base})).To(BeTrue())
		})
	})

	It("synchronizes copies through source and copy watch events", func() {
		const source = watchedSource
		ensureEnvironment()
		ref := &types.ProviderTLS{CABundleConfigMapRef: &types.CABundleConfigMapRef{Name: source, Key: "ca.crt"}}
		zoneA := newZone("trust-watch-a")
		zoneA.Spec.IdentityProviders[0].TLS = ref
		zoneB := newZone("trust-watch-b")
		zoneB.Spec.Gateways[0].TLS = ref
		for _, z := range []*adminv1.Zone{zoneA, zoneB} {
			Expect(k8sClient.Create(ctx, z)).To(Succeed())
			DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, z))).To(Succeed()) })
		}
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: source, Namespace: testEnvironment}}
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cm))).To(Succeed()) })

		trustReason := func(z *adminv1.Zone) func(Gomega) string {
			return func(g Gomega) string {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(z), z)).To(Succeed())
				c := meta.FindStatusCondition(z.Status.Conditions, zone_handler.ConditionTypeTrustSourcesReady)
				g.Expect(c).NotTo(BeNil())
				g.Expect(c.ObservedGeneration).To(Equal(z.Generation))
				return c.Reason
			}
		}

		By("blocking while the source is missing")
		Eventually(trustReason(zoneA), timeout, interval).Should(Equal(zone_handler.ReasonTrustSourceInvalid))

		By("keeping an unlabeled source out of the cache but visible to the API reader")
		cm.Data = map[string]string{"ca.crt": testCAPEM("a")}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		Consistently(func() bool {
			return apierrors.IsNotFound(k8sManager.GetClient().Get(ctx, client.ObjectKeyFromObject(cm), &corev1.ConfigMap{}))
		}, time.Second, interval).Should(BeTrue())
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(cm), &corev1.ConfigMap{})).To(Succeed())
		Expect(trustReason(zoneA)(Default)).To(Equal(zone_handler.ReasonTrustSourceInvalid))
		Expect(meta.FindStatusCondition(zoneA.Status.Conditions, zone_handler.ConditionTypeTrustSourcesReady).Message).
			To(ContainSubstring("not found or lacks label"), "the zone handler reads sources through the label-filtered cache")

		By("recovering every referencing zone once the label is added")
		cm.Labels = map[string]string{types.ProviderCALabelKey: "true"}
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())
		for _, z := range []*adminv1.Zone{zoneA, zoneB} {
			Eventually(trustReason(z), timeout, interval).Should(Equal(zone_handler.ReasonTrustSynchronized))
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, copyKey(z), &corev1.ConfigMap{})).To(Succeed())
			}, timeout, interval).Should(Succeed())
		}

		By("treating label removal as invalidation while keeping the copy")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)).To(Succeed())
		cm.Labels = nil
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())
		Eventually(trustReason(zoneA), timeout, interval).Should(Equal(zone_handler.ReasonTrustSourceInvalid))
		Expect(k8sClient.Get(ctx, copyKey(zoneA), &corev1.ConfigMap{})).To(Succeed())

		By("rotating the Zone bundle when the source is relabeled with new content")
		rotated := testCAPEM("b")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)).To(Succeed())
		cm.Labels = map[string]string{types.ProviderCALabelKey: "true"}
		cm.Data = map[string]string{"ca.crt": rotated}
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())
		Eventually(func(g Gomega) {
			cp := &corev1.ConfigMap{}
			g.Expect(k8sClient.Get(ctx, copyKey(zoneA), cp)).To(Succeed())
			g.Expect(cp.Data["ca.crt"]).To(Equal(rotated))
		}, timeout, interval).Should(Succeed())
		Eventually(trustReason(zoneA), timeout, interval).Should(Equal(zone_handler.ReasonTrustSynchronized))

		By("mapping a copy through its verified origin and repairing it")
		cp := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, copyKey(zoneA), cp)).To(Succeed())
		r := &ZoneReconciler{Client: k8sManager.GetClient()}
		Eventually(func() []reconcile.Request { return r.mapConfigMapToZones(ctx, cp) }, timeout, interval).
			Should(ContainElement(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(zoneA)}))
		Expect(k8sClient.Delete(ctx, cp)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, copyKey(zoneA), &corev1.ConfigMap{})).To(Succeed())
		}, timeout, interval).Should(Succeed())

		By("blocking with a relabel hint while the copy lacks the provider-ca label, and recovering after relabeling")
		Expect(k8sClient.Get(ctx, copyKey(zoneA), cp)).To(Succeed())
		delete(cp.Labels, types.ProviderCALabelKey)
		Expect(k8sClient.Update(ctx, cp)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zoneA), zoneA)).To(Succeed())
			c := meta.FindStatusCondition(zoneA.Status.Conditions, zone_handler.ConditionTypeTrustSourcesReady)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Reason).To(Equal(zone_handler.ReasonTrustCopyInvalid))
			g.Expect(c.Message).To(ContainSubstring("restore its label"))
		}, timeout, interval).Should(Succeed())
		Expect(k8sClient.Get(ctx, copyKey(zoneA), cp)).To(Succeed())
		cp.Labels[types.ProviderCALabelKey] = types.ProviderCALabelValue
		Expect(k8sClient.Update(ctx, cp)).To(Succeed())
		Eventually(trustReason(zoneA), timeout, interval).Should(Equal(zone_handler.ReasonTrustSynchronized))

		By("not identifying copies by the shared label alone")
		forged := cp.DeepCopy()
		forged.Namespace = testEnvironment
		forged.Annotations[providerca.AnnotationZoneUID] = "other"
		Expect(r.mapConfigMapToZones(ctx, forged)).NotTo(ContainElement(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(zoneA)}))
	})
})

var _ = Describe("Zone provider CA watch predicate and corrupted provenance", func() {
	base := func() *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "c", ResourceVersion: "1",
				Labels: map[string]string{
					types.ProviderCALabelKey: "true", config.EnvironmentLabelKey: testEnvironment,
					config.BuildLabelKey(zoneLabelName): "z", config.DomainLabelKey: domainName,
				},
				Annotations: map[string]string{providerca.AnnotationZoneUID: "u"},
			},
			Data: map[string]string{"ca.crt": "v"},
		}
	}
	update := func(mut func(*corev1.ConfigMap)) bool {
		o := base()
		n := o.DeepCopy()
		n.ResourceVersion = "2"
		mut(n)
		return ProviderCAConfigMapPredicate.Update(event.UpdateEvent{ObjectOld: o, ObjectNew: n})
	}

	DescribeTable("validity-relevant metadata passes, unrelated metadata does not",
		func(mut func(*corev1.ConfigMap), want bool) { Expect(update(mut)).To(Equal(want)) },
		Entry("environment label", func(c *corev1.ConfigMap) { c.Labels[config.EnvironmentLabelKey] = "other" }, true),
		Entry("zone label removed", func(c *corev1.ConfigMap) { delete(c.Labels, config.BuildLabelKey(zoneLabelName)) }, true),
		Entry("domain label", func(c *corev1.ConfigMap) { c.Labels[config.DomainLabelKey] = "x" }, true),
		Entry("admin provenance annotation", func(c *corev1.ConfigMap) { c.Annotations[providerca.AnnotationZoneUID] = "x" }, true),
		Entry("owner reference", func(c *corev1.ConfigMap) {
			c.OwnerReferences = []metav1.OwnerReference{{Name: "o", Kind: "K", APIVersion: "v1", UID: "x"}}
		}, true),
		Entry("unrelated label", func(c *corev1.ConfigMap) { c.Labels["team"] = "x" }, false),
		Entry("unrelated annotation", func(c *corev1.ConfigMap) { c.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "x" }, false),
	)

	It("does not map a copy whose provenance was corrupted and never adopts it", func() {
		ensureEnvironment()
		zone := newZone("corrupted-copy")
		zone.Spec.IdentityProviders[0].TLS = &types.ProviderTLS{CABundleConfigMapRef: &types.CABundleConfigMapRef{Name: "corrupted-src", Key: "ca.crt"}}
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed()) })
		src := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "corrupted-src", Namespace: testEnvironment, Labels: map[string]string{types.ProviderCALabelKey: "true"}},
			Data:       map[string]string{"ca.crt": testCAPEM("a")},
		}
		Expect(k8sClient.Create(ctx, src)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, src))).To(Succeed()) })

		var cp *corev1.ConfigMap
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)).To(Succeed())
			cp = &corev1.ConfigMap{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: zone.Status.Namespace, Name: zoneCopyName(zone)}, cp)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		By("corrupting the origin hash")
		cp.Annotations[providerca.AnnotationProviderCAOriginHash] = "corrupted"
		Expect(k8sClient.Update(ctx, cp)).To(Succeed())

		r := &ZoneReconciler{Client: k8sManager.GetClient()}
		Expect(r.mapConfigMapToZones(ctx, cp)).To(BeEmpty())

		Consistently(func(g Gomega) {
			current := &corev1.ConfigMap{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cp), current)).To(Succeed())
			g.Expect(current.Annotations[providerca.AnnotationProviderCAOriginHash]).To(Equal("corrupted"))
		}, time.Second, interval).Should(Succeed())
	})
})

// zoneCopyName is the name of the durable CA bundle of zone.
func zoneCopyName(zone *adminv1.Zone) string {
	o := providerca.ProviderCAOrigin{ZoneUID: string(zone.UID), ZoneNamespace: zone.Namespace, ZoneName: zone.Name}
	return o.CopyName()
}
