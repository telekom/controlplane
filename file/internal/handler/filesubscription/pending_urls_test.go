// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filesubscription

import (
	"context"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	cindex "github.com/telekom/controlplane/common/pkg/controller/index"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/index"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Pending file subscription endpoints", Ordered, func() {
	var k8sClient client.Client
	var scopedClient cclient.JanitorClient
	ctx := context.Background()

	BeforeAll(func() {
		testEnv := &envtest.Environment{ErrorIfCRDPathMissing: true}
		for _, module := range []string{"file", "sftp", "admin", "application", "approval"} {
			testEnv.CRDDirectoryPaths = append(testEnv.CRDDirectoryPaths,
				filepath.Join("..", "..", "..", "..", module, "config", "crd", "bases"))
		}
		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(testEnv.Stop()).To(Succeed()) })
		scheme := buildScheme()
		Expect(adminv1.AddToScheme(scheme)).To(Succeed())
		reader, err := cache.New(cfg, cache.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
		Expect(reader.IndexField(ctx, &filev1.FileType{}, index.FileTypeName, func(obj client.Object) []string {
			return []string{obj.GetName()}
		})).To(Succeed())
		Expect(reader.IndexField(ctx, &filev1.ZoneServiceConfig{}, index.FieldSpecZoneOnZoneServiceConfig, func(obj client.Object) []string {
			return []string{obj.(*filev1.ZoneServiceConfig).Spec.Zone.String()}
		})).To(Succeed())
		Expect(cindex.SetOwnerIndex(ctx, reader, &approvalv1.ApprovalRequest{})).To(Succeed())
		cacheCtx, cancel := context.WithCancel(ctx)
		stopped := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(stopped)
			Expect(reader.Start(cacheCtx)).To(Succeed())
		}()
		DeferCleanup(func() {
			cancel()
			Eventually(stopped, 5*time.Second, 10*time.Millisecond).Should(BeClosed())
		})
		Expect(reader.WaitForCacheSync(cacheCtx)).To(BeTrue())
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
		cachedClient, err := client.New(cfg, client.Options{Scheme: scheme, Cache: &client.CacheOptions{Reader: reader}})
		Expect(err).NotTo(HaveOccurred())
		scopedClient = cclient.NewJanitorClient(cclient.NewScopedClient(cachedClient, "test"))
	})

	It("persists provider-zone endpoints while the approval request remains pending", func() {
		metadata := func(name string) metav1.ObjectMeta {
			return metav1.ObjectMeta{
				Name: name, Namespace: "default",
				Labels: map[string]string{config.EnvironmentLabelKey: "test"},
			}
		}
		zone := &adminv1.Zone{ObjectMeta: metadata(testZoneName), Spec: adminv1.ZoneSpec{
			Visibility: adminv1.ZoneVisibilityWorld,
			Gateways: []adminv1.GatewayConfig{{
				Name: "gateway", Admin: adminv1.GatewayAdminConfig{IdentityProviderRef: "primary", Url: "https://gateway.example.com"},
			}},
			IdentityProviders: []adminv1.IdentityProviderConfig{{Name: "primary", IssuerHostname: "idp.example.com"}},
			Presets: []adminv1.Preset{{
				Name: "default", Type: adminv1.GatewayTypeAPI, Default: true, GatewayRef: "gateway", IdentityProviderRef: "primary",
				Urls: []adminv1.UrlConfig{{Hostname: "gateway.example.com", Scheme: "https", Port: 443, BasePath: "/"}},
			}},
		}}
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		zone.Status.Namespace = "default"
		Expect(k8sClient.Status().Update(ctx, zone)).To(Succeed())
		exposure := testFileExposure()
		exposure.ObjectMeta = metadata(testExposureName)
		exposure.Spec.Variant = "sftp"
		exposure.Spec.Zone.Namespace = "default"
		exposure.Spec.Provider.Namespace = "default"
		exposure.Spec.Approval.Strategy = filev1.ApprovalStrategySimple
		Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
		fileType := &filev1.FileType{ObjectMeta: metadata(testFileTypeName)}
		Expect(k8sClient.Create(ctx, fileType)).To(Succeed())
		fileType.Status.FileExposureRef = types.ObjectRefFromObject(exposure)
		Expect(k8sClient.Status().Update(ctx, fileType)).To(Succeed())
		for _, name := range []string{testRequestorAppName, testProviderAppName} {
			app := testApplication(name, name, "team@example.com")
			app.ObjectMeta = metadata(name)
			app.Spec.Secret = "test-secret-reference"
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			app.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Ready"))
			Expect(k8sClient.Status().Update(ctx, app)).To(Succeed())
		}
		service := testZoneServiceConfig()
		service.ObjectMeta = metadata(testZoneName)
		service.Spec.Zone = types.ObjectRefFromObject(zone)
		service.Spec.API = filev1.ManagedRouteConfig{Path: "/files", Url: "https://files.example.com"}
		Expect(k8sClient.Create(ctx, service)).To(Succeed())
		sub := testSubscription()
		sub.ObjectMeta = metadata(testSubscriptionName)
		sub.Spec.Zone = &types.ObjectRef{Name: "consumer-zone", Namespace: "default"}
		sub.Spec.Requestor.Namespace = "default"
		Expect(k8sClient.Create(ctx, sub)).To(Succeed())
		testCtx := cclient.WithClient(ctx, scopedClient)
		handler := &FileSubscriptionHandler{}
		Eventually(func(g Gomega) {
			g.Expect(handler.CreateOrUpdate(testCtx, sub)).To(Succeed())
			g.Expect(sub.Status.ServiceURL).To(Equal("sftp.internal:22"))
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
		Expect(sub.Status.ServiceExternalURL).To(Equal("sftp.external:2222"))
		Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Reason).To(Equal("ApprovalPending"))
		Expect(k8sClient.Status().Update(ctx, sub)).To(Succeed())
		persisted := &filev1.FileSubscription{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), persisted)).To(Succeed())
		Expect(persisted.Status.ServiceURL).To(Equal("sftp.internal:22"))
		Expect(persisted.Status.ServiceExternalURL).To(Equal("sftp.external:2222"))
		Expect(sub.Status.ApprovalRequest).NotTo(BeNil())
		request := &approvalv1.ApprovalRequest{}
		Expect(k8sClient.Get(ctx, sub.Status.ApprovalRequest.K8s(), request)).To(Succeed())
		Expect(request.Spec.State).To(Equal(approvalv1.ApprovalStatePending))
		sub.Status.ServiceURL = ""
		sub.Status.ServiceExternalURL = ""
		Expect(handler.CreateOrUpdate(testCtx, sub)).To(Succeed())
		Expect(sub.Status.ServiceURL).To(Equal("sftp.internal:22"))
		Expect(sub.Status.ServiceExternalURL).To(Equal("sftp.external:2222"))
		Expect(meta.IsStatusConditionFalse(sub.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
		users := &sftpv1.UserList{}
		Expect(k8sClient.List(ctx, users)).To(Succeed())
		Expect(users.Items).To(BeEmpty())

		Expect(k8sClient.Delete(ctx, service)).To(Succeed())
		Eventually(func() error {
			return handler.CreateOrUpdate(testCtx, sub)
		}, 5*time.Second, 100*time.Millisecond).Should(MatchError(ContainSubstring("expected exactly one ZoneServiceConfig")))
	})
})
