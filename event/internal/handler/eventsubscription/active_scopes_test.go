// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription_test

import (
	"context"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventsubscription"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Event active scopes", Ordered, func() {
	var db client.Client
	var obj *eventv1.EventSubscription
	var scopedDB client.Client
	ctx := context.Background()

	BeforeAll(func() {
		root := filepath.Join("..", "..", "..", "..")
		paths := make([]string, 0, 5)
		for _, module := range []string{"event", "admin", "application", "approval", "pubsub"} {
			paths = append(paths, filepath.Join(root, module, "config", "crd", "bases"))
		}
		testEnv := &envtest.Environment{CRDDirectoryPaths: paths, ErrorIfCRDPathMissing: true}
		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(testEnv.Stop()).To(Succeed()) })
		db, err = client.New(cfg, client.Options{Scheme: buildScheme()})
		Expect(err).NotTo(HaveOccurred())
		indexedCache, err := cache.New(cfg, cache.Options{Scheme: db.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		for _, child := range []client.Object{&approvalv1.ApprovalRequest{}, &pubsubv1.Subscriber{}} {
			Expect(index.SetOwnerIndex(ctx, indexedCache, child)).To(Succeed())
		}
		Expect(indexedCache.IndexField(ctx, &eventv1.EventConfig{}, ".spec.zone.name", func(resource client.Object) []string {
			return []string{resource.(*eventv1.EventConfig).Spec.Zone.Name}
		})).To(Succeed())
		cacheCtx, cancel := context.WithCancel(ctx)
		DeferCleanup(cancel)
		go func() { defer GinkgoRecover(); Expect(indexedCache.Start(cacheCtx)).To(Succeed()) }()
		Expect(indexedCache.WaitForCacheSync(ctx)).To(BeTrue())
		scopedDB = &scopeIndexedClient{Client: db, indexed: indexedCache}
		persist := func(resource ctypes.Object) {
			resource.SetLabels(map[string]string{
				config.EnvironmentLabelKey: "default",
				eventv1.EventTypeLabelKey:  labelutil.NormalizeLabelValue(testEventType),
			})
			resource.SetUID("")
			status := resource.DeepCopyObject().(ctypes.Object)
			Expect(db.Create(ctx, resource)).To(Succeed())
			status.SetResourceVersion(resource.GetResourceVersion())
			Expect(db.Status().Update(ctx, status)).To(Succeed())
		}
		et := makeReadyEventType(testEventType)
		persist(&et)
		zone := makeReadyZone("expo-zone", "default")
		zone.Spec.Gateway.Admin.Url = "https://admin.example.com"
		zone.Spec.Gateway.Presets = []adminv1.GatewayConfigPreset{{
			Name: "default", Default: true,
			Urls: []adminv1.UrlConfig{{Hostname: "gateway.example.com", BasePath: "/"}},
		}}
		zone.Spec.IdentityProvider.Url = "https://identity.example.com"
		zone.Status.Links = adminv1.Links{Url: "https://gateway.example.com", Issuer: "https://identity.example.com", LmsIssuer: "https://identity.example.com"}
		persist(zone)
		exposure := makeReadyEventExposure(testEventType)
		exposure.Spec.Approval.Strategy = eventv1.ApprovalStrategySimple
		exposure.Spec.Scopes = []eventv1.EventScope{{Name: "read"}, {Name: "write"}}
		persist(&exposure)
		ec := makeReadyEventConfig("expo-zone", true, nil)
		ec.Spec.Local = &eventv1.LocalBackend{
			Admin:              eventv1.AdminConfig{Url: "https://admin.example.com"},
			ServerSendEventUrl: "https://sse.example.com", PublishEventUrl: "https://publish.example.com",
		}
		ec.Status.CallbackURL = "https://gateway.example.com/callback"
		persist(&ec)
		Eventually(func(g Gomega) {
			configs := &eventv1.EventConfigList{}
			g.Expect(scopedDB.List(ctx, configs, client.MatchingFields{".spec.zone.name": "expo-zone"})).To(Succeed())
			g.Expect(configs.Items).To(HaveLen(1))
		}, 5*time.Second, 20*time.Millisecond).Should(Succeed())
		for _, name := range []string{"requestor-app", "provider-app"} {
			app := makeReadyApplication(name, name, name+"@example.com", name)
			app.Spec.Secret = "test-secret"
			persist(app)
		}
		obj = newEventSubscription("active-scopes", testEventType, "expo-zone")
		obj.UID = ""
		obj.Labels = map[string]string{config.EnvironmentLabelKey: "default"}
		obj.Spec.Scopes = []string{"read"}
		obj.Spec.Delivery.Callback = "https://consumer.example.com/callback"
		Expect(db.Create(ctx, obj)).To(Succeed())
	})

	reconcile := func() {
		Expect(db.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		c := cclient.NewJanitorClient(cclient.NewScopedClient(scopedDB, "default"))
		h := &eventsubscription.EventSubscriptionHandler{}
		Expect(h.CreateOrUpdate(cclient.WithClient(ctx, c), obj)).To(Succeed())
		Expect(db.Status().Update(ctx, obj)).To(Succeed())
	}

	It("tracks scope handoff before readiness, preserves pending changes, and clears revoked scopes", func() {
		reconcile()
		Expect(obj.Status.ActiveScopes).To(BeEmpty())
		Expect(obj.Status.ApprovalRequest).NotTo(BeNil(), obj.Status.Conditions)
		request := &approvalv1.ApprovalRequest{}
		Expect(db.Get(ctx, obj.Status.ApprovalRequest.K8s(), request)).To(Succeed())
		request.Spec.State = approvalv1.ApprovalStateGranted
		Expect(db.Update(ctx, request)).To(Succeed())
		approval := &approvalv1.Approval{
			ObjectMeta: metav1.ObjectMeta{Name: obj.Status.Approval.Name, Namespace: obj.Namespace, Labels: obj.Labels},
			Spec: approvalv1.ApprovalSpec{
				State: approvalv1.ApprovalStateGranted, Strategy: request.Spec.Strategy,
				Target: request.Spec.Target, Action: request.Spec.Action,
				Requester: request.Spec.Requester, Decider: request.Spec.Decider,
				ApprovedRequest: ctypes.ObjectRefFromObject(request),
			},
		}
		Expect(db.Create(ctx, approval)).To(Succeed())
		reconcile()
		Expect(obj.Status.ActiveScopes).To(Equal([]string{"read"}))
		subscriber := &pubsubv1.Subscriber{}
		Expect(db.Get(ctx, obj.Status.Subscriber.K8s(), subscriber)).To(Succeed())
		Expect(subscriber.Status.Conditions).To(BeEmpty())
		Expect(subscriber.Spec.AppliedScopes).To(Equal(obj.Status.ActiveScopes))

		previousRequest := obj.Status.ApprovalRequest.Name
		obj.Spec.Scopes = []string{"read", "write"}
		Expect(db.Update(ctx, obj)).To(Succeed())
		reconcile()
		Expect(obj.Status.ApprovalRequest.Name).NotTo(Equal(previousRequest))
		Expect(obj.Status.ActiveScopes).To(Equal([]string{"read"}))
		Expect(db.Get(ctx, obj.Status.ApprovalRequest.K8s(), request)).To(Succeed())
		request.Spec.State = approvalv1.ApprovalStateRejected
		Expect(db.Update(ctx, request)).To(Succeed())
		reconcile()
		Expect(obj.Status.ActiveScopes).To(Equal([]string{"read"}))
		Expect(db.Get(ctx, obj.Status.Subscriber.K8s(), subscriber)).To(Succeed())
		Expect(subscriber.Spec.AppliedScopes).To(Equal([]string{"read"}))

		approval.Spec.State = approvalv1.ApprovalStateSuspended
		Expect(db.Update(ctx, approval)).To(Succeed())
		Eventually(func(g Gomega) {
			children := &pubsubv1.SubscriberList{}
			g.Expect(scopedDB.List(ctx, children, cclient.OwnedBy(obj)...)).To(Succeed())
			g.Expect(children.Items).To(HaveLen(1))
		}, 5*time.Second, 20*time.Millisecond).Should(Succeed())
		reconcile()
		Expect(obj.Status.ActiveScopes).To(BeEmpty())
		subscribers := &pubsubv1.SubscriberList{}
		Expect(db.List(ctx, subscribers)).To(Succeed())
		Expect(subscribers.Items).To(BeEmpty())
	})
})

// scopeIndexedClient uses the informer index for owner-filtered lists and the API server for other reads.
type scopeIndexedClient struct {
	client.Client
	indexed client.Reader
}

func (c *scopeIndexedClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	options := &client.ListOptions{}
	options.ApplyOptions(opts)
	if options.FieldSelector != nil {
		return c.indexed.List(ctx, list, opts...)
	}
	return c.Client.List(ctx, list, opts...)
}
