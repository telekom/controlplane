// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	clientModel "github.com/telekom/controlplane/identity/internal/testutil/fixtures/client"
	identityproviderModel "github.com/telekom/controlplane/identity/internal/testutil/fixtures/identityprovider"
	realmModel "github.com/telekom/controlplane/identity/internal/testutil/fixtures/realm"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/test/tlsutil"
	"github.com/telekom/controlplane/identity/test/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const trustTimeout = 10 * time.Second

var trustNs int

func newTrustNamespace() string {
	trustNs++
	name := fmt.Sprintf("trust-%d", trustNs)
	Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
	return name
}

func newCAConfigMap(ns, name string, labeled bool) *ctypes.CABundleConfigMapRef {
	ca, err := tlsutil.NewCA(name)
	Expect(err).NotTo(HaveOccurred())
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Data: map[string]string{"ca.crt": ca.PEM}}
	if labeled {
		cm.Labels = map[string]string{ctypes.ProviderCALabelKey: ctypes.ProviderCALabelValue}
	}
	Expect(k8sClient.Create(context.Background(), cm)).To(Succeed())
	return &ctypes.CABundleConfigMapRef{Name: name, Key: "ca.crt"}
}

func setCALabel(ns, name string, labeled bool) {
	expireTrust()
	cm := &corev1.ConfigMap{}
	Expect(k8sClient.Get(context.Background(), k8sclient.ObjectKey{Namespace: ns, Name: name}, cm)).To(Succeed())
	cm.Labels = nil
	if labeled {
		cm.Labels = map[string]string{ctypes.ProviderCALabelKey: ctypes.ProviderCALabelValue}
	}
	Expect(k8sClient.Update(context.Background(), cm)).To(Succeed())
}

// nudge triggers a reconciliation through a label change, standing in for the periodic requeue:
// ConfigMap changes do not enqueue IdentityProviders, Realms or Clients.
func nudge(obj k8sclient.Object) {
	GinkgoHelper()
	Eventually(func() error {
		if err := k8sClient.Get(context.Background(), k8sclient.ObjectKeyFromObject(obj), obj); err != nil {
			return err
		}
		labels := obj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels["test.cp.ei.telekom.de/nudge"] = strconv.FormatInt(time.Now().UnixNano(), 10)
		obj.SetLabels(labels)
		return k8sClient.Update(context.Background(), obj)
	}, trustTimeout, interval).Should(Succeed())
}

func completeSnapshot(name string, ref *ctypes.CABundleConfigMapRef) identityv1.RealmStatus {
	adminURL := identityproviderModel.NewIdentityProviderSpec().AdminUrl
	s := identityv1.RealmStatus{
		IssuerUrl:     keycloak.DetermineIssuerUrlFrom(adminURL, name),
		AdminClientId: "admin-cli",
		AdminUserName: "admin",
		AdminPassword: "password",
		AdminUrl:      adminURL,
		AdminTokenUrl: keycloak.DetermineAdminTokenUrlFrom(adminURL, keycloak.MasterRealm),
	}
	if ref != nil {
		s.TLS = &ctypes.ProviderTLS{CABundleConfigMapRef: ref}
	}
	return s
}

// createBlockedRealm creates a Realm whose IdentityProvider is missing, so its status cannot
// advance, and stamps a historic connection snapshot on it.
func createBlockedRealm(ns, name string, ref *ctypes.CABundleConfigMapRef) *identityv1.Realm {
	ctx := context.Background()
	realm := realmModel.NewRealm(name, ns, testEnvironment, "missing-idp")
	Expect(k8sClient.Create(ctx, realm)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(realm), realm)).To(Succeed())
		g.Expect(realm.Finalizers).NotTo(BeEmpty())
		g.Expect(meta.IsStatusConditionFalse(realm.Status.Conditions, "Ready")).To(BeTrue())
	}, trustTimeout, interval).Should(Succeed())
	Eventually(func() error {
		if err := k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(realm), realm); err != nil {
			return err
		}
		conds := realm.Status.Conditions
		realm.Status = completeSnapshot(name, ref)
		realm.Status.Conditions = conds
		return k8sClient.Status().Update(ctx, realm)
	}, trustTimeout, interval).Should(Succeed())
	return realm
}

var _ = Describe("Provider trust", func() {
	ctx := context.Background()

	It("resolves trust only from provider-ca labeled ConfigMaps in the informer cache", func() {
		ns := newTrustNamespace()
		newCAConfigMap(ns, "labeled", true)
		newCAConfigMap(ns, "unlabeled", false)

		Eventually(func() error {
			return k8sManager.GetClient().Get(ctx, k8sclient.ObjectKey{Namespace: ns, Name: "labeled"}, &corev1.ConfigMap{})
		}, trustTimeout, interval).Should(Succeed())
		Consistently(func() bool {
			err := k8sManager.GetClient().Get(ctx, k8sclient.ObjectKey{Namespace: ns, Name: "unlabeled"}, &corev1.ConfigMap{})
			return apierrors.IsNotFound(err)
		}, time.Second, interval).Should(BeTrue())
		Expect(k8sManager.GetAPIReader().Get(ctx, k8sclient.ObjectKey{Namespace: ns, Name: "unlabeled"}, &corev1.ConfigMap{})).To(Succeed())

		By("resolving through the shared CA cache wired to the manager client")
		Expect(keycloak.ValidateProviderCA(ctx, caCache, ns, &ctypes.CABundleConfigMapRef{Name: "labeled", Key: "ca.crt"})).To(Succeed())
		err := keycloak.ValidateProviderCA(ctx, caCache, ns, &ctypes.CABundleConfigMapRef{Name: "unlabeled", Key: "ca.crt"})
		Expect(err).To(MatchError(ContainSubstring("not found or lacks label")))
	})

	It("persists the trust snapshot and reports TrustReady on IdentityProvider and Realm", func() {
		ns := newTrustNamespace()
		ref := newCAConfigMap(ns, "idp-ca", true)
		idp := identityproviderModel.NewIdentityProvider("idp", ns, testEnvironment)
		idp.Spec.TLS = &ctypes.ProviderTLS{CABundleConfigMapRef: ref}
		Expect(k8sClient.Create(ctx, idp)).To(Succeed())
		realm := realmModel.NewRealm(utils.Realm, ns, testEnvironment, "idp")
		Expect(k8sClient.Create(ctx, realm)).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(realm), realm)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(realm.Status.Conditions, "Ready")).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(realm.Status.Conditions, identityv1.TrustReadyConditionType)).To(BeTrue())
			g.Expect(realm.Status.TLS.Ref()).To(Equal(ref))
		}, trustTimeout, interval).Should(Succeed())

		By("removing the provider-ca label: the next reconciliation observes it")
		setCALabel(ns, "idp-ca", false)
		nudge(idp)
		nudge(realm)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(idp), idp)).To(Succeed())
			g.Expect(meta.IsStatusConditionFalse(idp.Status.Conditions, identityv1.TrustReadyConditionType)).To(BeTrue())
			g.Expect(k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(realm), realm)).To(Succeed())
			g.Expect(meta.IsStatusConditionFalse(realm.Status.Conditions, "Ready")).To(BeTrue())
			g.Expect(meta.IsStatusConditionFalse(realm.Status.Conditions, identityv1.TrustReadyConditionType)).To(BeTrue(),
				"the Realm reports its own committed trust while its IdentityProvider is not ready")
		}, trustTimeout, interval).Should(Succeed())
		Expect(realm.Status.TLS.Ref()).To(Equal(ref), "the committed snapshot is retained")

		By("restoring the label: the next reconciliation recovers")
		setCALabel(ns, "idp-ca", true)
		nudge(idp)
		nudge(realm)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(realm), realm)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(realm.Status.Conditions, "Ready")).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(realm.Status.Conditions, identityv1.TrustReadyConditionType)).To(BeTrue())
		}, trustTimeout, interval).Should(Succeed())
	})

	It("reports the Realm's own valid committed trust while its IdentityProvider is missing", func() {
		ns := newTrustNamespace()
		ref := newCAConfigMap(ns, "hist-ca", true)
		realm := createBlockedRealm(ns, "own-trust-realm", ref)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(realm), realm)).To(Succeed())
			g.Expect(meta.IsStatusConditionFalse(realm.Status.Conditions, "Ready")).To(BeTrue())
			c := meta.FindStatusCondition(realm.Status.Conditions, identityv1.TrustReadyConditionType)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(c.ObservedGeneration).To(Equal(realm.Generation))
		}, trustTimeout, interval).Should(Succeed())

		By("losing the label flips the Realm's TrustReady on its next reconciliation")
		setCALabel(ns, "hist-ca", false)
		nudge(realm)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(realm), realm)).To(Succeed())
			g.Expect(meta.IsStatusConditionFalse(realm.Status.Conditions, identityv1.TrustReadyConditionType)).To(BeTrue())
		}, trustTimeout, interval).Should(Succeed())
	})

	It("completes cleanup of a terminating Realm on its next reconciliation after trust repair", func() {
		ns := newTrustNamespace()
		ref := newCAConfigMap(ns, "hist-ca", false)
		realm := createBlockedRealm(ns, utils.Realm, ref)

		By("deleting while the historic CA is invalid: the finalizer is retained")
		Expect(k8sClient.Delete(ctx, realm)).To(Succeed())
		Consistently(func() error {
			return k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(realm), &identityv1.Realm{})
		}, 2*time.Second, interval).Should(Succeed())

		By("labeling the CA: the next reconciliation of the terminating Realm completes cleanup")
		setCALabel(ns, "hist-ca", true)
		nudge(realm)
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(realm), &identityv1.Realm{}))
		}, trustTimeout, interval).Should(BeTrue())
	})

	It("deletes a Client through a not-ready Realm's persisted snapshot", func() {
		ns := newTrustNamespace()
		createBlockedRealm(ns, utils.RealmForClient, nil)
		cl := clientModel.NewClient("notready-client", ns, testEnvironment, utils.RealmForClient)
		Expect(k8sClient.Create(ctx, cl)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(cl), cl)).To(Succeed())
			g.Expect(cl.Finalizers).NotTo(BeEmpty())
		}, trustTimeout, interval).Should(Succeed())

		Expect(k8sClient.Delete(ctx, cl)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, k8sclient.ObjectKeyFromObject(cl), &identityv1.Client{}))
		}, trustTimeout, interval).Should(BeTrue())
	})
})
