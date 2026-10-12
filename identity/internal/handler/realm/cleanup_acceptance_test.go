// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/providerca"
	common "github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/test/tlsutil"
	secrets "github.com/telekom/controlplane/secret-manager/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Production factory + generic controller deletion through historic Realm snapshots that
// reference a retained Admin CA copy. No IdentityProvider, source ConfigMap or Zone exists.
var _ = Describe("Historic Realm cleanup through a retained Admin CA copy", func() {
	It("uses the rotated copy for every historic Realm once cached trust expires", func() {
		ctx := context.Background()
		ns := newNamespace()
		original := secrets.Get
		secrets.Get = func(_ context.Context, ref string) (string, error) { return ref, nil }
		DeferCleanup(func() { secrets.Get = original })

		caA, err := tlsutil.NewCA("ca-a")
		Expect(err).NotTo(HaveOccurred())
		caB, err := tlsutil.NewCA("ca-b")
		Expect(err).NotTo(HaveOccurred())

		var tokenCalls atomic.Int32
		token, err := tlsutil.NewRotatingTLSServer(caA, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			tokenCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "token_type": "bearer", "expires_in": 300})
		}))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(token.Close)

		var mu sync.Mutex
		var deleted []string
		api, err := tlsutil.NewRotatingTLSServer(caA, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			deleted = append(deleted, r.Method+" "+r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		}))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(api.Close)

		By("retaining an Admin copy with valid provenance and CA A")
		origin := providerca.ProviderCAOrigin{
			ZoneUID: "zone-uid", ZoneNamespace: "zone-ns", ZoneName: "zone-a", ConfigMap: "source", Key: "ca.crt",
		}
		copyCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: origin.CopyName(), Namespace: ns, Annotations: origin.Annotations(),
				Labels: map[string]string{
					common.ProviderCALabelKey:     common.ProviderCALabelValue,
					cconfig.EnvironmentLabelKey:   testEnv_,
					cconfig.BuildLabelKey("zone"): "zone-a",
				},
			},
			Data: map[string]string{providerca.ProviderCACopyKey: caA.PEM},
		}
		Expect(k8sClient.Create(ctx, copyCM)).To(Succeed())
		ref := &common.CABundleConfigMapRef{Name: origin.CopyName(), Key: providerca.ProviderCACopyKey}

		By("creating two historic Realms that share the copy and have no live IdentityProvider")
		adminURL := api.URL + "/admin/realms/"
		var clockOffset atomic.Int64
		caCache := providerca.NewCache(k8sClient, commonclient.ParseProviderCABundle, providerca.WithClock(func() time.Time {
			return time.Now().Add(time.Duration(clockOffset.Load()))
		}))
		clients := keycloak.NewClientCache(caCache)
		DeferCleanup(clients.Close)
		factory := keycloak.NewServiceFactory(clients)
		names := []string{"historic-1", "historic-2"}
		for _, name := range names {
			realm := &identityv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name: name, Namespace: ns, Labels: envLabels(), Finalizers: []string{cconfig.FinalizerName},
				},
				Spec: identityv1.RealmSpec{IdentityProvider: &common.ObjectRef{Namespace: ns, Name: "gone-idp"}},
			}
			Expect(k8sClient.Create(ctx, realm)).To(Succeed())
			realm.Status = identityv1.RealmStatus{
				IssuerUrl:     token.URL + "/realms/" + name,
				AdminClientId: "admin-cli", AdminUserName: "admin", AdminPassword: literalSecret,
				AdminUrl:      adminURL,
				AdminTokenUrl: token.URL + "/realms/master/protocol/openid-connect/token",
				TLS:           &common.ProviderTLS{CABundleConfigMapRef: ref},
			}
			Expect(k8sClient.Status().Update(ctx, realm)).To(Succeed())

			By("priming the factory cache with CA A for " + name)
			_, err := factory.ServiceFor(ctx, realm)
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(tokenCalls.Load()).To(Equal(int32(2)))

		By("rotating the retained copy to CA B only (same-source rotation)")
		Expect(k8sClient.Get(ctx, pkgclient.ObjectKeyFromObject(copyCM), copyCM)).To(Succeed())
		copyCM.Data[providerca.ProviderCACopyKey] = caB.PEM
		Expect(k8sClient.Update(ctx, copyCM)).To(Succeed())
		clockOffset.Add(int64(time.Minute))

		controller := cc.NewController(NewHandlerRealm(factory, caCache), k8sClient, record.NewFakeRecorder(100))
		reconcileRealm := func(name string) {
			_, _ = controller.Reconcile(ctx, reconcile.Request{NamespacedName: pkgclient.ObjectKey{Namespace: ns, Name: name}}, &identityv1.Realm{})
		}
		for _, name := range names {
			Expect(k8sClient.Delete(ctx, &identityv1.Realm{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}})).To(Succeed())
		}

		By("failing cleanup while the servers still present CA A: cached CA A clients are not reused")
		reconcileRealm(names[0])
		stuck := &identityv1.Realm{}
		Expect(k8sClient.Get(ctx, pkgclient.ObjectKey{Namespace: ns, Name: names[0]}, stuck)).To(Succeed())
		Expect(stuck.Finalizers).To(ContainElement(cconfig.FinalizerName))
		mu.Lock()
		Expect(deleted).To(BeEmpty())
		mu.Unlock()

		By("rotating the server certificates to CA B: cleanup reaches the historic token and API endpoints")
		Expect(token.Rotate(caB)).To(Succeed())
		Expect(api.Rotate(caB)).To(Succeed())
		for _, name := range names {
			Eventually(func() bool {
				reconcileRealm(name)
				return apierrors.IsNotFound(k8sClient.Get(ctx, pkgclient.ObjectKey{Namespace: ns, Name: name}, &identityv1.Realm{}))
			}).WithTimeout(15 * time.Second).WithPolling(100 * time.Millisecond).Should(BeTrue())
		}
		mu.Lock()
		defer mu.Unlock()
		Expect(deleted).To(ConsistOf("DELETE /admin/realms/historic-1", "DELETE /admin/realms/historic-2"))
	})
})
