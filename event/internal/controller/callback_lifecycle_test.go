// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventconfig"
	"github.com/telekom/controlplane/event/internal/handler/eventsubscription"
	"github.com/telekom/controlplane/event/internal/index"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type callbackIndexedClient struct {
	client.Client
	reader cache.Cache
}

func (c callbackIndexedClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.reader.List(ctx, list, opts...)
}

// Envtest persists routes, statuses and Subscribers; an indexed controller-runtime
// cache supplies the same field selectors used by the real handlers. External
// gateway, approval and pubsub controllers are not running.
var _ = Describe("callback lifecycle with persisted resources", func() {
	const env = "callback-lifecycle"
	labels := map[string]string{cconfig.EnvironmentLabelKey: env}
	key := func(name string) types.NamespacedName { return types.NamespacedName{Name: name, Namespace: "default"} }
	ref := func(name string) ctypes.ObjectRef { return ctypes.ObjectRef{Name: name, Namespace: "default"} }
	ready := func(conditions *[]metav1.Condition, generation int64) {
		meta.SetStatusCondition(conditions, metav1.Condition{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Provisioned", ObservedGeneration: generation})
	}
	create := func(obj client.Object) {
		ExpectWithOffset(1, k8sClient.Create(ctx, obj)).To(Succeed())
		DeferCleanup(func() {
			err := k8sClient.Delete(ctx, obj)
			ExpectWithOffset(1, err == nil || errors.IsNotFound(err)).To(BeTrue())
		})
	}
	store := func(obj client.Object) { ExpectWithOffset(1, k8sClient.Status().Update(ctx, obj)).To(Succeed()) }
	lookup := func(zone string) *eventv1.EventConfig {
		list := &eventv1.EventConfigList{}
		Expect(k8sClient.List(ctx, list, client.MatchingLabels{cconfig.EnvironmentLabelKey: env})).To(Succeed())
		for i := range list.Items {
			if list.Items[i].Spec.Zone.Name == zone {
				return &list.Items[i]
			}
		}
		Fail("EventConfig missing for zone " + zone)
		return nil
	}
	makeConfig := func(name string, proxy bool, mesh []string) *eventv1.EventConfig {
		cfg := &eventv1.EventConfig{ObjectMeta: metav1.ObjectMeta{Name: name + "-callback-lifecycle", Namespace: "default", Labels: labels}, Spec: eventv1.EventConfigSpec{Zone: ref(name), Mesh: &eventv1.MeshConfig{ZoneNames: mesh, Client: eventv1.ClientConfig{ClientId: name + "-mesh", Realm: ref("callback-realm")}}}}
		if proxy {
			cfg.Spec.Proxy = &eventv1.ProxyBackend{TargetZone: ref("backend")}
		} else {
			cfg.Spec.Local = &eventv1.LocalBackend{Admin: eventv1.AdminConfig{Url: "https://admin.example.com", Client: eventv1.ClientConfig{ClientId: name + "-admin", ClientSecret: "test-secret", Realm: ref("callback-realm")}}, ServerSendEventUrl: "https://sse.example.com", PublishEventUrl: "https://publish.example.com"}
		}
		create(cfg)
		return cfg
	}
	makeZone := func(name string) {
		idpAdminUrl := "https://idp-admin.example.com"
		zone := &adminv1.Zone{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: labels}, Spec: adminv1.ZoneSpec{
			Visibility:        adminv1.ZoneVisibilityEnterprise,
			IdentityProviders: []adminv1.IdentityProviderConfig{{Name: "default", IssuerHostname: "idp.example.com", Admin: adminv1.IdentityProviderAdminConfig{Url: &idpAdminUrl}}},
			Gateways:          []adminv1.GatewayConfig{{Name: "default", Admin: adminv1.GatewayAdminConfig{Url: "https://gateway-admin.example.com"}}},
			Presets:           []adminv1.Preset{{Name: "event", Type: adminv1.GatewayTypeEvent, Default: true, GatewayRef: "default", IdentityProviderRef: "default", Urls: []adminv1.UrlConfig{{Scheme: "https", Hostname: name + ".example.com", Port: 443, BasePath: "/"}}}},
		}}
		create(zone)
		zone.Status.Namespace = "default"
		zone.Status.IdentityRealm = &ctypes.ObjectRef{Name: "callback-realm", Namespace: "default"}
		zone.Status.InternalIdentityRealm = &ctypes.ObjectRef{Name: "callback-realm", Namespace: "default"}
		zone.Status.Presets = []adminv1.PresetStatus{{
			Name:       "event",
			GatewayRef: &ctypes.ObjectRef{Name: name + "-gateway", Namespace: "default"},
			Links: adminv1.Links{
				Url:       "https://" + name + ".example.com",
				Issuer:    "https://" + name + "-issuer",
				LmsIssuer: "https://" + name + "-lms/spacegate",
			},
		}}
		ready(&zone.Status.Conditions, zone.Generation)
		store(zone)
	}
	makeApp := func(name string) {
		app := &applicationv1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: labels}, Spec: applicationv1.ApplicationSpec{Team: name, TeamEmail: name + "@example.com", Secret: "test-secret"}}
		create(app)
		app.Status.ClientId = name
		ready(&app.Status.Conditions, app.Generation)
		store(app)
	}
	newIndexedClient := func() callbackIndexedClient {
		cacheCtx, stopCache := context.WithCancel(ctx)
		startErrors := make(chan error, 1)
		DeferCleanup(func() {
			stopCache()
			Expect(<-startErrors).To(Succeed())
		})
		reader, err := cache.New(cfg, cache.Options{Scheme: k8sClient.Scheme()})
		Expect(err).To(Succeed())
		Expect(index.RegisterEventSubscriptionMapperIndices(cacheCtx, reader)).To(Succeed())
		Expect(reader.IndexField(cacheCtx, &eventv1.EventConfig{}, index.EventConfigZoneIndex, func(obj client.Object) []string {
			return []string{obj.(*eventv1.EventConfig).Spec.Zone.Name}
		})).To(Succeed())
		for _, obj := range []client.Object{&pubsubv1.Subscriber{}, &pubsubv1.EventStore{}, &identityv1.Client{}, &gatewayv1.Consumer{}, &gatewayv1.Route{}, &approvalv1.ApprovalRequest{}} {
			Expect(reader.IndexField(cacheCtx, obj, ".metadata.controller", func(obj client.Object) []string {
				for _, owner := range obj.GetOwnerReferences() {
					if owner.Controller != nil && *owner.Controller {
						return []string{string(owner.UID)}
					}
				}
				return nil
			})).To(Succeed())
		}
		go func() { startErrors <- reader.Start(cacheCtx) }()
		Expect(reader.WaitForCacheSync(cacheCtx)).To(BeTrue())
		return callbackIndexedClient{Client: k8sClient, reader: reader}
	}

	DescribeTable("manages callback Consumers with matching callback ACLs", func(isProxy bool, clientId string) {
		indexedClient := newIndexedClient()
		namespace := "callback-consumer-local"
		if isProxy {
			namespace = "callback-consumer-proxy"
		}
		create(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
		moveZone := func(name string) {
			makeZone(name)
			zone := &adminv1.Zone{}
			Expect(k8sClient.Get(ctx, key(name), zone)).To(Succeed())
			zone.Status.Namespace = namespace
			zone.Status.Presets[0].GatewayRef.Namespace = namespace
			store(zone)
		}
		moveConfig := func(cfg *eventv1.EventConfig) {
			Expect(k8sClient.Delete(ctx, cfg)).To(Succeed())
			// The fixture helper creates configs in default; move them into the
			// correlated zone namespace before reconciliation.
			cfg.ResourceVersion = ""
			cfg.UID = ""
			cfg.Namespace = namespace
			cfg.Spec.Zone.Namespace = "default"
			Expect(k8sClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, cfg)).To(Succeed()) })
		}
		moveZone("backend")
		backend := makeConfig("backend", false, nil)
		moveConfig(backend)
		cfg := backend
		if isProxy {
			moveZone("exposure")
			cfg = makeConfig("exposure", true, nil)
			moveConfig(cfg)
		}
		realm := &identityv1.Realm{ObjectMeta: metav1.ObjectMeta{Name: "callback-realm", Namespace: "default", Labels: labels}, Spec: identityv1.RealmSpec{IdentityProvider: &ctypes.ObjectRef{Name: "callback-idp", Namespace: "default"}}}
		create(realm)
		realm.Status.IssuerUrl = "https://issuer.example.com"
		store(realm)
		// The proxy EventStore reads the target admin client configuration.
		ready(&backend.Status.Conditions, backend.Generation)
		store(backend)

		h := &eventconfig.EventConfigHandler{}
		reconcile := func(obj *eventv1.EventConfig) {
			ExpectWithOffset(1, h.CreateOrUpdate(cclient.WithClient(contextutil.WithEnv(ctx, env), cclient.NewJanitorClient(cclient.NewScopedClient(indexedClient, env))), obj)).To(Succeed())
		}
		cfg.Spec.Mesh.Client.ClientId = clientId
		reconcile(cfg)
		consumer := &gatewayv1.Consumer{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: clientId, Namespace: namespace}, consumer)).To(Succeed())
		Expect(consumer.Spec.Name).To(Equal(clientId))
		identityClient := &identityv1.Client{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: clientId, Namespace: namespace}, identityClient)).To(Succeed())
		Expect(identityClient.Spec.ClientId).To(Equal(consumer.Spec.Name))
		Expect(consumer.Spec.Gateway).To(Equal(ctypes.ObjectRef{Name: cfg.Spec.Zone.Name + "-gateway", Namespace: namespace}))
		Expect(consumer.OwnerReferences).To(ContainElement(SatisfyAll(HaveField("UID", cfg.UID), HaveField("Kind", "EventConfig"))))
		callback := &gatewayv1.Route{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "callback--" + cfg.Spec.Zone.Name, Namespace: namespace}, callback)).To(Succeed())
		Expect(callback.Spec.Security.DefaultConsumers).To(ContainElement(consumer.Spec.Name))
		Expect(callback.Spec.Security.DefaultConsumers).NotTo(ContainElement("backend-mesh"))
		cfg.Spec.Mesh.Client.ClientSecret = "rotated"
		reconcile(cfg)
		updated := &gatewayv1.Consumer{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: clientId, Namespace: namespace}, updated)).To(Succeed())
		Expect(updated.UID).To(Equal(consumer.UID))
		Expect(updated.Spec).To(Equal(consumer.Spec))
		if !isProxy {
			cfg.Spec.Mesh.Client.ClientId = "replacement-callback"
			reconcile(cfg)
			replacement := &gatewayv1.Consumer{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "replacement-callback", Namespace: namespace}, replacement)).To(Succeed())
			Expect(replacement.Spec.Name).To(Equal("replacement-callback"))
			Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: clientId, Namespace: namespace}, &gatewayv1.Consumer{}))).To(BeTrue())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "callback--backend", Namespace: namespace}, callback)).To(Succeed())
			Expect(callback.Spec.Security.DefaultConsumers).To(ContainElement("replacement-callback"))
			Expect(callback.Spec.Security.DefaultConsumers).NotTo(ContainElement(clientId))
		}
	},
		Entry("local zone with custom client", false, "custom-backend-callback"),
		Entry("proxy zone with default client", true, "eventstore"),
	)

	DescribeTable("does not claim an existing gateway Consumer", func(clientId, ownership string) {
		namespace := "callback-collision-" + ownership
		if clientId == gatewayv1.GatewayConsumerName {
			namespace = "callback-collision-gateway"
		}
		create(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
		makeZone("collision-zone")
		zone := &adminv1.Zone{}
		Expect(k8sClient.Get(ctx, key("collision-zone"), zone)).To(Succeed())
		zone.Status.Namespace = namespace
		zone.Status.Presets[0].GatewayRef = &ctypes.ObjectRef{Name: "zone-gateway", Namespace: zone.Status.Namespace}
		zone.Status.IdentityRealm = &ctypes.ObjectRef{Name: "callback-collision-realm", Namespace: "default"}
		store(zone)
		realm := &identityv1.Realm{ObjectMeta: metav1.ObjectMeta{Name: "callback-collision-realm", Namespace: "default", Labels: labels}, Spec: identityv1.RealmSpec{IdentityProvider: &ctypes.ObjectRef{Name: "callback-idp", Namespace: "default"}}}
		create(realm)
		cfg := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "collision-config", Namespace: zone.Status.Namespace, Labels: labels},
			Spec: eventv1.EventConfigSpec{
				Zone:  ref(zone.Name),
				Proxy: &eventv1.ProxyBackend{TargetZone: ref("unused")},
				Mesh:  &eventv1.MeshConfig{Client: eventv1.ClientConfig{ClientId: clientId, Realm: ref(realm.Name)}},
			},
		}
		create(cfg)
		original := &gatewayv1.Consumer{
			ObjectMeta: metav1.ObjectMeta{Name: clientId, Namespace: cfg.Namespace, Labels: map[string]string{cconfig.EnvironmentLabelKey: env, cconfig.DomainLabelKey: "event"}},
			Spec:       gatewayv1.ConsumerSpec{Gateway: *zone.Status.Presets[0].GatewayRef, Name: "existing-name"},
		}
		if ownership == "foreign" {
			other := &eventv1.EventConfig{ObjectMeta: metav1.ObjectMeta{Name: "other-config", Namespace: cfg.Namespace, UID: "other-uid"}}
			Expect(controllerutil.SetControllerReference(other, original, k8sClient.Scheme())).To(Succeed())
		}
		create(original)
		before := original.DeepCopy()

		h := &eventconfig.EventConfigHandler{}
		err := h.CreateOrUpdate(cclient.WithClient(contextutil.WithEnv(ctx, env), cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, env))), cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("gateway Consumer"))
		persisted := &gatewayv1.Consumer{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(original), persisted)).To(Succeed())
		Expect(persisted.UID).To(Equal(before.UID))
		Expect(persisted.Spec).To(Equal(before.Spec))
		Expect(persisted.Labels).To(Equal(before.Labels))
		Expect(persisted.OwnerReferences).To(Equal(before.OwnerReferences))
	},
		Entry("unowned callback Consumer", "eventstore", "unowned"),
		Entry("Consumer controlled by another config", "eventstore", "foreign"),
		Entry("reserved zone gateway Consumer", gatewayv1.GatewayConsumerName, "unowned"),
	)

	It("cleans up obsolete owned callback forwards while retaining primary and unrelated routes", func() {
		indexedClient := newIndexedClient()
		for _, name := range []string{"backend-clean", "exposure-clean", "subscriber-clean"} {
			makeZone(name)
		}
		backend := makeConfig("backend-clean", false, []string{"exposure-clean", "subscriber-clean"})
		proxy := makeConfig("exposure-clean", true, []string{"subscriber-clean"})
		create(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "callback-cleanup"}})
		zone := &adminv1.Zone{}
		Expect(k8sClient.Get(ctx, key("exposure-clean"), zone)).To(Succeed())
		zone.Status.Namespace = "callback-cleanup"
		zone.Status.Presets[0].GatewayRef.Namespace = "callback-cleanup"
		store(zone)
		Expect(k8sClient.Delete(ctx, proxy)).To(Succeed())
		proxy.ResourceVersion = ""
		proxy.UID = ""
		proxy.Namespace = "callback-cleanup"
		Expect(k8sClient.Create(ctx, proxy)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, proxy)).To(Succeed()) })
		proxy.Spec.Proxy.TargetZone = ref("backend-clean")
		Expect(k8sClient.Update(ctx, proxy)).To(Succeed())
		makeConfig("subscriber-clean", false, nil)
		realm := &identityv1.Realm{ObjectMeta: metav1.ObjectMeta{Name: "callback-realm", Namespace: "default", Labels: labels}, Spec: identityv1.RealmSpec{IdentityProvider: &ctypes.ObjectRef{Name: "callback-idp", Namespace: "default"}}}
		create(realm)
		realm.Status.IssuerUrl = "https://issuer.example.com"
		store(realm)
		backend.Status.CallbackURL = "https://backend-clean.example.com/horizon-backend-clean/callback/v1"
		backend.Status.ProxyCallbackURLs = map[string]string{"exposure-clean": "https://backend-clean.example.com/horizon-exposure-clean/callback/v1", "subscriber-clean": "https://backend-clean.example.com/horizon-subscriber-clean/callback/v1"}
		ready(&backend.Status.Conditions, backend.Generation)
		store(backend)

		obsolete := &gatewayv1.Route{ObjectMeta: metav1.ObjectMeta{Name: "callback--subscriber-clean", Namespace: proxy.Namespace, Labels: labels}, Spec: gatewayv1.RouteSpec{GatewayRef: ref("exposure-clean-gateway"), Type: gatewayv1.RouteTypeProxy, Backend: gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{{Scheme: "https", Hostname: "subscriber-clean.example.com", Port: 443, Path: "/horizon-subscriber-clean/callback/v1"}}}, Hostnames: []string{"exposure-clean.example.com"}, Paths: []string{"/horizon-subscriber-clean/callback/v1"}}}
		Expect(controllerutil.SetControllerReference(proxy, obsolete, k8sClient.Scheme())).To(Succeed())
		create(obsolete)
		foreign := obsolete.DeepCopy()
		foreign.Name = "foreign-callback-lifecycle"
		foreign.ResourceVersion = ""
		foreign.UID = ""
		foreign.OwnerReferences = nil
		otherOwner := &eventv1.EventConfig{ObjectMeta: metav1.ObjectMeta{Name: "other-cleanup", Namespace: proxy.Namespace, UID: "foreign-cleanup-uid"}}
		Expect(controllerutil.SetControllerReference(otherOwner, foreign, k8sClient.Scheme())).To(Succeed())
		create(foreign)
		// The cache must see the old route before the janitor lists owned children.
		Eventually(func(g Gomega) {
			listed := &gatewayv1.RouteList{}
			g.Expect(indexedClient.List(ctx, listed, cclient.OwnedBy(proxy)...)).To(Succeed())
			g.Expect(listed.Items).To(ContainElement(HaveField("Name", obsolete.Name)))
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(proxy), proxy)).To(Succeed())
		Expect((&eventconfig.EventConfigHandler{}).CreateOrUpdate(cclient.WithClient(contextutil.WithEnv(ctx, env), cclient.NewJanitorClient(cclient.NewScopedClient(indexedClient, env))), proxy)).To(Succeed())
		Expect(proxy.Status.ProxyCallbackRoutes).To(BeEmpty())
		Expect(proxy.Status.CallbackRoute).NotTo(BeNil())
		Eventually(func() bool {
			return errors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(obsolete), &gatewayv1.Route{}))
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "callback--exposure-clean", Namespace: proxy.Namespace}, &gatewayv1.Route{})).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(foreign), &gatewayv1.Route{})).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "publish", Namespace: proxy.Namespace}, &gatewayv1.Route{})).To(Succeed())

		// Switching to a real backend materializes the outbound callback Route;
		// switching back removes it without removing the primary.
		proxy.Spec.Proxy = nil
		proxy.Spec.Local = backend.Spec.Local.DeepCopy()
		proxy.Spec.Local.Admin.Client.ClientId = "exposure-clean-admin"
		Expect(k8sClient.Update(ctx, proxy)).To(Succeed())
		proxy = &eventv1.EventConfig{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "exposure-clean-callback-lifecycle", Namespace: "callback-cleanup"}, proxy)).To(Succeed())
		reconcile := func() {
			ExpectWithOffset(1, (&eventconfig.EventConfigHandler{}).CreateOrUpdate(cclient.WithClient(contextutil.WithEnv(ctx, env), cclient.NewJanitorClient(cclient.NewScopedClient(indexedClient, env))), proxy)).To(Succeed())
		}
		reconcile()
		Expect(proxy.Status.ProxyCallbackRoutes).To(HaveLen(1))
		forward := &gatewayv1.Route{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "callback--subscriber-clean", Namespace: proxy.Namespace}, forward)).To(Succeed())
		Expect(forward.Spec.Security.TrustedIssuers).To(ConsistOf("https://exposure-clean-issuer"))
		proxy.Spec.Local = nil
		proxy.Spec.Proxy = &eventv1.ProxyBackend{TargetZone: ref("backend-clean")}
		Expect(k8sClient.Update(ctx, proxy)).To(Succeed())
		proxy = &eventv1.EventConfig{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "exposure-clean-callback-lifecycle", Namespace: "callback-cleanup"}, proxy)).To(Succeed())
		reconcile()
		Expect(proxy.Status.ProxyCallbackRoutes).To(BeEmpty())
		Eventually(func() bool {
			return errors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(forward), &gatewayv1.Route{}))
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "callback--exposure-clean", Namespace: proxy.Namespace}, &gatewayv1.Route{})).To(Succeed())
	})

	It("projects backend changes through proxy status into callback provisioning, revocation and restoration", func() {
		indexedClient := newIndexedClient()
		for _, name := range []string{"backend", "exposure", "subscriber"} {
			makeZone(name)
		}
		backend := makeConfig("backend", false, []string{"exposure", "subscriber"})
		proxy := makeConfig("exposure", true, []string{"subscriber"})
		subscriberCfg := makeConfig("subscriber", false, nil)
		realm := &identityv1.Realm{ObjectMeta: metav1.ObjectMeta{Name: "callback-realm", Namespace: "default", Labels: labels}, Spec: identityv1.RealmSpec{IdentityProvider: &ctypes.ObjectRef{Name: "callback-idp", Namespace: "default"}}}
		create(realm)
		realm.Status.IssuerUrl = "https://issuer.example.com"
		store(realm)
		makeApp("callback-requestor")
		makeApp("callback-provider")

		backend.Status.CallbackURL = "https://backend.example.com/horizon-backend/callback/v1"
		backend.Status.ProxyCallbackURLs = map[string]string{"exposure": "https://backend.example.com/horizon-exposure/callback/v1", "subscriber": "https://backend.example.com/horizon-subscriber/callback/v1"}
		ready(&backend.Status.Conditions, backend.Generation)
		store(backend)
		subscriberCfg.Status.CallbackRoute = &ctypes.ObjectRef{Name: "subscriber-primary", Namespace: "default"}
		subscriberCfg.Status.CallbackURL = "" // The primary ref, not the effective URL, gates final delivery.
		ready(&subscriberCfg.Status.Conditions, subscriberCfg.Generation)
		store(subscriberCfg)

		configCtx := contextutil.WithEnv(ctx, env)
		configHandler := &eventconfig.EventConfigHandler{}
		// The handler persists owned routes and resolves the ready target through the API.
		buildProxyCallbacks := func() {
			proxy = lookup("exposure")
			before := proxy.DeepCopy().Status
			Expect(configHandler.CreateOrUpdate(cclient.WithClient(configCtx, cclient.NewJanitorClient(cclient.NewScopedClient(indexedClient, env))), proxy)).To(Succeed())
			ready(&proxy.Status.Conditions, proxy.Generation)
			if !apiequality.Semantic.DeepEqual(before, proxy.Status) {
				store(proxy)
			}
		}
		buildProxyCallbacks()
		Expect(proxy.Status.CallbackURL).To(Equal(backend.Status.ProxyCallbackURLs["exposure"]))
		Expect(proxy.Status.ProxyCallbackURLs).To(HaveKeyWithValue("subscriber", backend.Status.ProxyCallbackURLs["subscriber"]))
		convergedRV := proxy.ResourceVersion
		buildProxyCallbacks()
		Expect(proxy.ResourceVersion).To(Equal(convergedRV))

		eventType := &eventv1.EventType{ObjectMeta: metav1.ObjectMeta{Name: eventv1.MakeEventTypeName("de.telekom.callbacklifecycle.v1"), Namespace: "default", Labels: labels}, Spec: eventv1.EventTypeSpec{Type: "de.telekom.callbacklifecycle.v1", Version: "1.0.0"}}
		create(eventType)
		eventType.Status.Active = true
		ready(&eventType.Status.Conditions, eventType.Generation)
		store(eventType)
		exposure := &eventv1.EventExposure{ObjectMeta: metav1.ObjectMeta{Name: "callback-lifecycle-exposure", Namespace: "default", Labels: map[string]string{cconfig.EnvironmentLabelKey: env, eventv1.EventTypeLabelKey: labelutil.NormalizeLabelValue(eventType.Spec.Type)}}, Spec: eventv1.EventExposureSpec{EventType: eventType.Spec.Type, Zone: ref("exposure"), Visibility: eventv1.VisibilityEnterprise, Provider: ctypes.TypedObjectRef{ObjectRef: ref("callback-provider")}, Approval: eventv1.Approval{Strategy: eventv1.ApprovalStrategyAuto}}}
		create(exposure)
		exposure.Status.Active = true
		exposure.Status.Publisher = &ctypes.ObjectRef{Name: "callback-publisher", Namespace: "default"}
		ready(&exposure.Status.Conditions, exposure.Generation)
		store(exposure)
		sub := &eventv1.EventSubscription{ObjectMeta: metav1.ObjectMeta{Name: "callback-lifecycle-subscription", Namespace: "default", Labels: labels}, Spec: eventv1.EventSubscriptionSpec{EventType: eventType.Spec.Type, Zone: ref("subscriber"), Requestor: ctypes.TypedObjectRef{TypeMeta: metav1.TypeMeta{Kind: "Application"}, ObjectRef: ref("callback-requestor")}, Delivery: eventv1.Delivery{Type: eventv1.DeliveryTypeCallback, Payload: eventv1.PayloadTypeData, Callback: "https://consumer.example.com/events"}}}
		create(sub)
		recorder := record.NewFakeRecorder(100)
		r := &EventSubscriptionReconciler{Client: indexedClient}
		r.Controller = cc.NewController(&eventsubscription.EventSubscriptionHandler{}, indexedClient, recorder)
		reconcileSub := func() {
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(sub)})
			Expect(err).To(Succeed())
		}
		child := &pubsubv1.Subscriber{}
		childKey := key(sub.Name)
		// First setup and approval request are real; the approval controller is
		// not running in this deterministic test, so grant its persisted decision.
		reconcileSub()
		reconcileSub()
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
		approval := &approvalv1.Approval{ObjectMeta: metav1.ObjectMeta{Name: sub.Status.Approval.Name, Namespace: "default", Labels: labels}, Spec: approvalv1.ApprovalSpec{State: approvalv1.ApprovalStateGranted, Strategy: approvalv1.ApprovalStrategyAuto, Target: sub.Spec.Requestor}}
		create(approval)
		reconcileSub()
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, childKey, child)).To(Succeed())
			g.Expect(child.Spec.Delivery.Callback).To(Equal(backend.Status.ProxyCallbackURLs["subscriber"] + "?callback=https://consumer.example.com/events"))
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		oldScalar := proxy.Status.CallbackURL
		backend.Status.ProxyCallbackURLs["subscriber"] = "https://backend.example.com/horizon-subscriber/new-callback/v1"
		store(backend)
		configMapper := &EventConfigReconciler{Client: indexedClient}
		Expect(configMapper.MapEventConfigToEventConfig(ctx, backend)).To(ContainElement(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(proxy)}))
		previousRV := proxy.ResourceVersion
		buildProxyCallbacks()
		Expect(proxy.ResourceVersion).NotTo(Equal(previousRV))
		Expect(proxy.Status.CallbackURL).To(Equal(oldScalar))
		Expect(proxy.Status.ProxyCallbackURLs["subscriber"]).To(Equal(backend.Status.ProxyCallbackURLs["subscriber"]))
		persistedProxy := lookup("exposure")
		Expect(persistedProxy.Status.ProxyCallbackURLs).To(HaveKeyWithValue("subscriber", backend.Status.ProxyCallbackURLs["subscriber"]))
		Eventually(func() []reconcile.Request { return r.MapEventConfigToEventSubscription(ctx, proxy) }, 5*time.Second, 100*time.Millisecond).Should(ContainElement(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(sub)}))
		reconcileSub()
		Expect(k8sClient.Get(ctx, childKey, child)).To(Succeed())
		Expect(child.Spec.Delivery.Callback).To(Equal(backend.Status.ProxyCallbackURLs["subscriber"] + "?callback=https://consumer.example.com/events"))

		proxy.Spec.Mesh.ZoneNames = []string{"backend"}
		Expect(k8sClient.Update(ctx, proxy)).To(Succeed())
		buildProxyCallbacks()
		Expect(proxy.Status.ProxyCallbackURLs).NotTo(HaveKey("subscriber"))
		Expect(lookup("backend").Status.ProxyCallbackURLs).To(HaveKey("subscriber"))
		reconcileSub()
		Eventually(func() bool {
			err := k8sClient.Get(ctx, childKey, &pubsubv1.Subscriber{})
			return errors.IsNotFound(err)
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(), fmt.Sprintf("callback Subscriber %s was not deleted after mesh revocation", childKey))
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
		Expect(meta.IsStatusConditionFalse(sub.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		reconcileSub()
		Expect(errors.IsNotFound(k8sClient.Get(ctx, childKey, &pubsubv1.Subscriber{}))).To(BeTrue())

		proxy.Spec.Mesh.ZoneNames = []string{"subscriber"}
		Expect(k8sClient.Update(ctx, proxy)).To(Succeed())
		buildProxyCallbacks()
		reconcileSub()
		Expect(k8sClient.Get(ctx, childKey, child)).To(Succeed())

		// An unready backend removes effective ingress without treating this as
		// an explicit logical-mesh revocation of the existing Subscriber.
		backend = lookup("backend")
		meta.SetStatusCondition(&backend.Status.Conditions, metav1.Condition{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "Unavailable"})
		store(backend)
		proxy = lookup("exposure")
		Expect(configHandler.CreateOrUpdate(cclient.WithClient(configCtx, cclient.NewJanitorClient(cclient.NewScopedClient(indexedClient, env))), proxy)).To(HaveOccurred())
		Expect(proxy.Status.CallbackURL).To(BeEmpty())
		store(proxy)
		Expect(lookup("exposure").Status.ProxyCallbackURLs).To(BeEmpty())
		reconcileSub() // Backend readiness failure must not deprovision an existing child.
		Expect(k8sClient.Get(ctx, childKey, child)).To(Succeed())
		ready(&backend.Status.Conditions, backend.Generation)
		store(backend)
		buildProxyCallbacks()
		Expect(proxy.Status.CallbackURL).To(Equal(backend.Status.ProxyCallbackURLs["exposure"]))

		// Retargeting to a missing config is likewise blocked; recovering the
		// target recomputes from the currently selected backend, not stale URLs.
		proxy.Spec.Proxy.TargetZone = ref("missing-backend")
		Expect(k8sClient.Update(ctx, proxy)).To(Succeed())
		proxy = lookup("exposure")
		Expect(configHandler.CreateOrUpdate(cclient.WithClient(configCtx, cclient.NewJanitorClient(cclient.NewScopedClient(indexedClient, env))), proxy)).To(HaveOccurred())
		Expect(proxy.Status.CallbackURL).To(BeEmpty())
		store(proxy)
		Expect(lookup("exposure").Status.ProxyCallbackURLs).To(BeEmpty())
		proxy.Spec.Proxy.TargetZone = ref("backend")
		Expect(k8sClient.Update(ctx, proxy)).To(Succeed())
		buildProxyCallbacks()
		Expect(proxy.Status.ProxyCallbackURLs).To(HaveKeyWithValue("subscriber", backend.Status.ProxyCallbackURLs["subscriber"]))
	})
})
