// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticsubscription_test

import (
	"context"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/agenticsubscription"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Agentic active scopes", Ordered, func() {
	var testEnv *envtest.Environment
	var db client.Client
	var obj *agenticv1.AgenticSubscription
	var scopedDB client.Client
	ctx := context.Background()

	BeforeAll(func() {
		s := buildScheme()
		Expect(gatewayv1.AddToScheme(s)).To(Succeed())
		root := filepath.Join("..", "..", "..", "..")
		paths := make([]string, 0, 5)
		for _, module := range []string{"agentic", "admin", "application", "approval", "gateway"} {
			paths = append(paths, filepath.Join(root, module, "config", "crd", "bases"))
		}
		testEnv = &envtest.Environment{CRDDirectoryPaths: paths, ErrorIfCRDPathMissing: true}
		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(testEnv.Stop()).To(Succeed()) })
		db, err = client.New(cfg, client.Options{Scheme: s})
		Expect(err).NotTo(HaveOccurred())
		indexedCache, err := cache.New(cfg, cache.Options{Scheme: s})
		Expect(err).NotTo(HaveOccurred())
		for _, child := range []client.Object{&approvalv1.ApprovalRequest{}, &gatewayv1.ConsumeRoute{}} {
			Expect(index.SetOwnerIndex(ctx, indexedCache, child)).To(Succeed())
		}
		cacheCtx, cancel := context.WithCancel(ctx)
		DeferCleanup(cancel)
		go func() { defer GinkgoRecover(); Expect(indexedCache.Start(cacheCtx)).To(Succeed()) }()
		Expect(indexedCache.WaitForCacheSync(ctx)).To(BeTrue())
		scopedDB = &scopeIndexedClient{Client: db, indexed: indexedCache}

		persist := func(resource ctypes.Object) {
			labels := resource.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels[config.EnvironmentLabelKey] = "default"
			resource.SetLabels(labels)
			resource.SetUID("")
			status := resource.DeepCopyObject().(ctypes.Object)
			Expect(db.Create(ctx, resource)).To(Succeed())
			status.SetResourceVersion(resource.GetResourceVersion())
			Expect(db.Status().Update(ctx, status)).To(Succeed())
		}
		server := makeReadyMcpServer("/mcp/weather/v1")
		server.Spec.Oauth2Scopes = []string{"read", "write"}
		server.Labels = map[string]string{agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(server.Spec.BasePath)}
		persist(&server)
		exposure := makeReadyAgenticExposure(server.Spec.BasePath, "test-zone")
		exposure.Spec.Upstreams = []agenticv1.Upstream{{Url: "https://example.com"}}
		exposure.Spec.Approval.Strategy = agenticv1.ApprovalStrategySimple
		exposure.Labels = map[string]string{agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(server.Spec.BasePath)}
		persist(&exposure)
		zone := makeReadyZoneWithAiGateway("test-zone")
		zone.Spec.Visibility = adminv1.ZoneVisibilityEnterprise
		zone.Spec.AiGateway.Admin.Url = "https://admin.example.com"
		zone.Spec.AiGateway.Presets[0].Urls[0].BasePath = "/"
		zone.Spec.Gateway.Admin.Url = "https://admin.example.com"
		zone.Spec.Gateway.Presets = zone.Spec.AiGateway.Presets
		zone.Spec.IdentityProvider.Url = "https://identity.example.com"
		zone.Status.Links.Url = "https://gateway.example.com"
		zone.Status.Links.LmsIssuer = "https://issuer.example.com"
		persist(zone)
		for _, name := range []string{"requestor-app", "provider-app"} {
			app := makeReadyApplication(name, name, name+"@example.com", name)
			app.Spec.Secret = "test-secret"
			persist(app)
		}
		obj = newAgenticSubscription("active-scopes", server.Spec.BasePath, "test-zone")
		obj.UID = ""
		obj.Labels = map[string]string{config.EnvironmentLabelKey: "default"}
		obj.Spec.Security = &agenticv1.SubscriberSecurity{M2M: &agenticv1.SubscriberMachine2MachineAuthentication{Scopes: []string{"read"}}}
		Expect(db.Create(ctx, obj)).To(Succeed())
	})

	reconcile := func() {
		Expect(db.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		c := cclient.NewJanitorClient(cclient.NewScopedClient(scopedDB, "default"))
		h := &agenticsubscription.AgenticSubscriptionHandler{}
		Expect(h.CreateOrUpdate(cclient.WithClient(ctx, c), obj)).To(Succeed())
		Expect(db.Status().Update(ctx, obj)).To(Succeed())
	}

	grant := func() *approvalv1.Approval {
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
		return approval
	}

	It("tracks provisioned scopes before child readiness and preserves them across pending and rejected changes", func() {
		reconcile()
		Expect(obj.Status.ActiveScopes).To(BeEmpty())
		approval := grant()
		reconcile()
		Expect(obj.Status.ActiveScopes).To(Equal([]string{"read"}))
		route := &gatewayv1.ConsumeRoute{}
		Expect(db.Get(ctx, obj.Status.ConsumeRoute.K8s(), route)).To(Succeed())
		Expect(route.Status.Conditions).To(BeEmpty())
		Expect(route.Spec.Security.M2M.Scopes).To(Equal(obj.Status.ActiveScopes))

		oldRequest := obj.Status.ApprovalRequest.Name
		obj.Spec.Security.M2M.Scopes = []string{"read", "write"}
		Expect(db.Update(ctx, obj)).To(Succeed())
		reconcile()
		Expect(obj.Status.ApprovalRequest.Name).NotTo(Equal(oldRequest))
		Expect(obj.Status.ActiveScopes).To(Equal([]string{"read"}))
		request := &approvalv1.ApprovalRequest{}
		Expect(db.Get(ctx, obj.Status.ApprovalRequest.K8s(), request)).To(Succeed())
		request.Spec.State = approvalv1.ApprovalStateRejected
		Expect(db.Update(ctx, request)).To(Succeed())
		reconcile()
		Expect(obj.Status.ActiveScopes).To(Equal([]string{"read"}))
		Expect(db.Get(ctx, obj.Status.ConsumeRoute.K8s(), route)).To(Succeed())
		Expect(route.Spec.Security.M2M.Scopes).To(Equal([]string{"read"}))

		approval.Spec.State = approvalv1.ApprovalStateSuspended
		Expect(db.Update(ctx, approval)).To(Succeed())
		Eventually(func(g Gomega) {
			children := &gatewayv1.ConsumeRouteList{}
			g.Expect(scopedDB.List(ctx, children, cclient.OwnedBy(obj)...)).To(Succeed())
			g.Expect(children.Items).To(HaveLen(1))
		}, 5*time.Second, 20*time.Millisecond).Should(Succeed())
		reconcile()
		Expect(obj.Status.ActiveScopes).To(BeEmpty())
		routes := &gatewayv1.ConsumeRouteList{}
		Expect(db.List(ctx, routes)).To(Succeed())
		Expect(routes.Items).To(BeEmpty())
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
