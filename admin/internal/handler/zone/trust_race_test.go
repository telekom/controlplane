// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"errors"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/providerca"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// hookClient runs a hook once, right before the first Get of the given key, against the real API
// server. It interleaves a concurrent change between staging and mutation.
type hookClient struct {
	client.Client
	key  client.ObjectKey
	once sync.Once
	hook func()
}

func (c *hookClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key == c.key {
		c.once.Do(c.hook)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func objectMeta(key client.ObjectKey) metav1.ObjectMeta {
	return metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name}
}

func isRetryable(err error) bool {
	var ce *ctrlerrors.CtrlError
	return errors.As(err, &ce) && ce.IsRetryable()
}

var _ = Describe("Zone publication races", func() {
	var zone *adminv1.Zone
	var idx int

	BeforeEach(func() {
		idx++
		zone = newTestZone(fmt.Sprintf("race-zone-%d", idx))
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed()) })
	})

	idpKey := func() client.ObjectKey {
		return client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForIdentityProvider(zone, "primary")}
	}
	gwKey := func() client.ObjectKey {
		return client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForGateway(zone, "standard")}
	}

	provision := func(h *ZoneHandler) {
		GinkgoHelper()
		Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		markSubResourcesReady(zone)
		Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
	}

	// runInterleaved runs the handler with a scoped client that changes the API state right before the
	// publication's fetch of key, i.e. after the handler staged its candidate from a direct read.
	runInterleaved := func(h *ZoneHandler, key client.ObjectKey, hook func()) error {
		return h.CreateOrUpdate(newTestContextWithClient(zone, &hookClient{Client: k8sClient, key: key, hook: hook}), zone)
	}

	replaceWithForeign := func(obj client.Object, mutate func(client.Object)) {
		GinkgoHelper()
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
		obj.SetResourceVersion("")
		obj.SetUID("")
		obj.SetAnnotations(map[string]string{providerca.AnnotationZoneUID: "foreign-uid"})
		mutate(obj)
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())
	}

	It("does not overwrite an IdentityProvider replaced by another Zone incarnation after staging", func() {
		h := newTrustHandler()
		provision(h)
		newURL := "https://moved-iris.de/auth/admin/realms"
		zone.Spec.IdentityProviders[0].Admin.Url = &newURL

		err := runInterleaved(h, idpKey(), func() {
			replaceWithForeign(&identityapi.IdentityProvider{ObjectMeta: objectMeta(idpKey())}, func(o client.Object) {
				o.(*identityapi.IdentityProvider).Spec.AdminUrl = "https://foreign.de/auth/admin/realms"
			})
		})
		Expect(isRetryable(err)).To(BeTrue(), "%v", err)
		idp := getIDP(zone)
		Expect(idp.Annotations[providerca.AnnotationZoneUID]).To(Equal("foreign-uid"))
		Expect(idp.Spec.AdminUrl).To(Equal("https://foreign.de/auth/admin/realms"))

		By("restaging on the next reconciliation rejects the foreign child")
		err = h.CreateOrUpdate(newTestContext(zone), zone)
		Expect(isBlocked(err)).To(BeTrue(), "%v", err)
		Expect(getIDP(zone).Spec.AdminUrl).To(Equal("https://foreign.de/auth/admin/realms"))
	})

	It("does not overwrite an IdentityProvider configuration changed after staging", func() {
		h := newTrustHandler()
		provision(h)
		err := runInterleaved(h, idpKey(), func() {
			idp := getIDP(zone)
			idp.Spec.AdminPassword = "changed-concurrently"
			Expect(k8sClient.Update(ctx, idp)).To(Succeed())
		})
		Expect(isRetryable(err)).To(BeTrue(), "%v", err)
		Expect(getIDP(zone).Spec.AdminPassword).To(Equal("changed-concurrently"))

		By("converging from the restaged state afterwards")
		Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(getIDP(zone).Spec.AdminPassword).To(Equal("test-idp-admin-password"))
	})

	It("does not create an IdentityProvider that appeared after staging", func() {
		h := newTrustHandler()
		err := runInterleaved(h, idpKey(), func() {
			idp := &identityapi.IdentityProvider{ObjectMeta: objectMeta(idpKey())}
			idp.Labels = map[string]string{"cp.ei.telekom.de/environment": testEnvironment}
			idp.Annotations = map[string]string{providerca.AnnotationZoneUID: "foreign-uid"}
			idp.Spec.AdminUrl = "https://foreign.de/auth/admin/realms"
			Expect(k8sClient.Create(ctx, idp)).To(Succeed())
		})
		Expect(isRetryable(err)).To(BeTrue(), "%v", err)
		Expect(getIDP(zone).Annotations[providerca.AnnotationZoneUID]).To(Equal("foreign-uid"))
	})

	It("does not overwrite a Gateway replaced after staging", func() {
		h := newTrustHandler()
		provision(h)
		zone.Spec.Gateways[0].Admin.Url = "https://test-stargate.de/new-admin-api"
		err := runInterleaved(h, gwKey(), func() {
			replaceWithForeign(&gatewayapi.Gateway{ObjectMeta: objectMeta(gwKey())}, func(o client.Object) {
				o.(*gatewayapi.Gateway).Spec.Admin.Url = "https://foreign-stargate.de/admin-api"
			})
		})
		Expect(isRetryable(err)).To(BeTrue(), "%v", err)
		gw := getGateway(zone)
		Expect(gw.Annotations[providerca.AnnotationZoneUID]).To(Equal("foreign-uid"))
		Expect(gw.Spec.Admin.Url).To(Equal("https://foreign-stargate.de/admin-api"))
	})

	Context("Zone deletion observed during reconciliation", func() {
		deleteZone := func() {
			Expect(k8sClient.Delete(ctx, zone.DeepCopy())).To(Succeed())
		}

		It("stops after a copy write is refused and publishes no provider", func() {
			source := fmt.Sprintf("race-src-%d", idx)
			DeferCleanup(func() { deleteSource(source) })
			putSource(source, newTestCAPEM("a"), true)
			zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
			origin := zoneOrigin(zone)
			copyKey := client.ObjectKey{Namespace: zoneNamespace(zone), Name: origin.CopyName()}
			h := NewZoneHandler(&hookClient{Client: k8sClient, key: copyKey, hook: deleteZone}, k8sClient)
			useSecretManager()

			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, copyKey, &corev1.ConfigMap{}))).To(BeTrue())
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, idpKey(), &identityapi.IdentityProvider{}))).To(BeTrue())
		})

		It("publishes no provider for a zone without references", func() {
			h := NewZoneHandler(&hookClient{Client: k8sClient, key: idpKey(), hook: deleteZone}, k8sClient)
			useSecretManager()
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, idpKey(), &identityapi.IdentityProvider{}))).To(BeTrue())
		})

		It("does not publish an explicit reference removal after deletion is observed", func() {
			source := fmt.Sprintf("race-src-%d", idx)
			DeferCleanup(func() { deleteSource(source) })
			putSource(source, newTestCAPEM("a"), true)
			zone.Spec.IdentityProviders[0].TLS = tlsRef(source)
			Expect(k8sClient.Update(ctx, zone)).To(Succeed())
			provision(newTrustHandler())
			published := getIDP(zone).Spec.TLS.Ref()
			Expect(published).NotTo(BeNil())

			zone.Spec.IdentityProviders[0].TLS = nil
			h := NewZoneHandler(&hookClient{Client: k8sClient, key: idpKey(), hook: deleteZone}, k8sClient)
			Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			Expect(getIDP(zone).Spec.TLS.Ref()).To(Equal(published))
		})
	})
})

var _ = Describe("Gateway loop after an observed Zone deletion", func() {
	It("stops at the first Gateway write and writes no later Gateway or Consumer", func() {
		zone := newTestZone("race-gw-stop")
		zone.Spec.Gateways = append(zone.Spec.Gateways, adminv1.GatewayConfig{
			Name: "second", Admin: adminv1.GatewayAdminConfig{IdentityProviderRef: "primary", Url: "https://second.example.com/admin-api"},
		})
		zone.Spec.Presets = append(zone.Spec.Presets, adminv1.Preset{
			Name: "ai", Type: adminv1.GatewayTypeAI, Default: true, GatewayRef: "second", IdentityProviderRef: "primary",
			Urls: []adminv1.UrlConfig{{Hostname: "second.example.com", BasePath: "/"}},
		})
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed()) })

		h := newTrustHandler()
		Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		markSubResourcesReady(zone)
		Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())

		zone.Spec.Gateways[0].Admin.Url = "https://test-stargate.de/new-admin-api"
		secondConsumerKey := client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForGatewayConsumer(zone, "second")}
		Expect(k8sClient.Delete(ctx, &gatewayapi.Consumer{ObjectMeta: objectMeta(secondConsumerKey)})).To(Succeed())
		oldStandard := getGateway(zone)

		testCtx := newTestContext(zone)
		hc := newTestHandlingContext(testCtx, zone)
		hc.Handler = h
		Expect(h.prepareProviders(testCtx, hc)).To(Succeed())
		for _, step := range []Step{createIdentityProvider, createDefaultIdentityRealm, createInternalIdentityRealm} {
			Expect(step(testCtx, hc)).To(Succeed())
		}

		By("deleting the Zone when the first Gateway write checks liveness")
		h.Reader = &hookClient{Client: k8sClient, key: client.ObjectKeyFromObject(zone), hook: func() {
			Expect(k8sClient.Delete(ctx, zone.DeepCopy())).To(Succeed())
		}}
		err := reconcileGateways(testCtx, hc)
		Expect(errors.Is(err, errZoneNotLive)).To(BeTrue(), "%v", err)

		Expect(getGateway(zone).Spec.Admin).To(Equal(oldStandard.Spec.Admin), "the Gateway change was not written")
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, secondConsumerKey, &gatewayapi.Consumer{}))).To(BeTrue(),
			"no Consumer of a later Gateway is written after the stop")
	})
})

var _ = DescribeTable("Shared Realm snapshot classification (identity/api/v1)",
	func(mut func(*identityapi.RealmStatus), want identityapi.RealmSnapshotState) {
		s := &identityapi.RealmStatus{}
		mut(s)
		Expect(identityapi.ClassifyRealmSnapshot(s)).To(Equal(want))
	},
	Entry("empty", func(*identityapi.RealmStatus) {}, identityapi.RealmSnapshotEmpty),
	Entry("TLS only", func(s *identityapi.RealmStatus) {
		s.TLS = &types.ProviderTLS{CABundleConfigMapRef: &types.CABundleConfigMapRef{Name: "c", Key: "k"}}
	}, identityapi.RealmSnapshotPartial),
	Entry("credentials only", func(s *identityapi.RealmStatus) { s.AdminClientId, s.AdminUserName, s.AdminPassword = "a", "b", "c" }, identityapi.RealmSnapshotPartial),
	Entry("URLs without credentials", func(s *identityapi.RealmStatus) { s.IssuerUrl, s.AdminUrl, s.AdminTokenUrl = "a", "b", "c" }, identityapi.RealmSnapshotPartial),
	Entry("all but password", func(s *identityapi.RealmStatus) {
		s.IssuerUrl, s.AdminUrl, s.AdminTokenUrl, s.AdminClientId, s.AdminUserName = "a", "b", "c", "d", "e"
	}, identityapi.RealmSnapshotPartial),
	Entry("complete", func(s *identityapi.RealmStatus) {
		s.IssuerUrl, s.AdminUrl, s.AdminTokenUrl, s.AdminClientId, s.AdminUserName, s.AdminPassword = "a", "b", "c", "d", "e", "f"
	}, identityapi.RealmSnapshotComplete),
)

// conditionalDeleteClient deletes the Zone on the first Zone read at which cond holds.
type conditionalDeleteClient struct {
	client.Client
	zone    *adminv1.Zone
	cond    func() bool
	deleted bool
}

func (c *conditionalDeleteClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, isZone := obj.(*adminv1.Zone); isZone && key == client.ObjectKeyFromObject(c.zone) && !c.deleted && c.cond() {
		c.deleted = true
		Expect(c.Client.Delete(ctx, c.zone.DeepCopy())).To(Succeed())
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

var _ = Describe("Zone deletion observed at a step boundary", func() {
	It("stops before the next child-writing step", func() {
		zone := newTestZone("boundary-stop")
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed()) })

		h := newTrustHandler()
		idpKey := client.ObjectKey{Namespace: zoneNamespace(zone), Name: naming.ForIdentityProvider(zone, "primary")}
		h.Reader = &conditionalDeleteClient{Client: k8sClient, zone: zone, cond: func() bool {
			return k8sClient.Get(ctx, idpKey, &identityapi.IdentityProvider{}) == nil
		}}

		Expect(h.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed(), "an observed deletion ends reconciliation quietly")
		Expect(k8sClient.Get(ctx, idpKey, &identityapi.IdentityProvider{})).To(Succeed())
		realms := &identityapi.RealmList{}
		Expect(k8sClient.List(ctx, realms, client.InNamespace(zoneNamespace(zone)))).To(Succeed())
		Expect(realms.Items).To(BeEmpty(), "no Realm is written after deletion was observed")
	})
})
