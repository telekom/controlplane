// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/telekom/controlplane/common/pkg/test/mock"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	features_mock "github.com/telekom/controlplane/gateway/internal/features/mock"
	kong_client "github.com/telekom/controlplane/gateway/pkg/kong/client"
	kong_clientmock "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	timeout         = 2 * time.Second
	interval        = 100 * time.Millisecond
	testNamespace   = "default"
	testEnvironment = "test"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc

var GetMockClientFor func(gwCfg kongutil.GatewayAdminConfig) *kong_clientmock.MockKongClient

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("%s-%s-%s", os.Getenv("ENVTEST_K8S_VERSION"), runtime.GOOS, runtime.GOARCH)),
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = gatewayv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Creating the manager")
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	Expect(err).ToNot(HaveOccurred())

	By("Registering all required indices")
	RegisterIndecesOrDie(ctx, k8sManager)

	By("Setting up controllers")
	err = (&GatewayReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&RealmReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&RouteReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ConsumerReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ConsumeRouteReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("Creating the environment namespace")
	CreateNamespace(testEnvironment)

	By("Setting up the required mocks")
	mockCtrl := gomock.NewController(GinkgoT())
	clientCacheMutex := sync.Mutex{}
	kongClientMockCache := make(map[string]*kong_clientmock.MockKongClient)

	kongutil.GetClientFor = func(gwCfg kongutil.GatewayAdminConfig) (kong_client.KongClient, error) {
		clientCacheMutex.Lock()
		defer clientCacheMutex.Unlock()
		if client, found := kongClientMockCache[gwCfg.AdminUrl()]; found {
			return client, nil
		}
		client := kong_clientmock.NewMockKongClient(mockCtrl)
		kongClientMockCache[gwCfg.AdminUrl()] = client
		return client, nil
	}

	features.NewFeatureBuilder = func(kc kong_client.KongClient, route *gatewayv1.Route, consumer *gatewayv1.Consumer, realm *gatewayv1.Realm, gateway *gatewayv1.Gateway) features.FeaturesBuilder {
		mockBuilder := features_mock.NewMockFeaturesBuilder(mockCtrl)
		mockBuilder.EXPECT().EnableFeature(gomock.Any()).MinTimes(1)
		mockBuilder.EXPECT().AddAllowedConsumers(gomock.Any()).AnyTimes()
		mockBuilder.EXPECT().Build(gomock.Any()).Return(nil).AnyTimes()
		mockBuilder.EXPECT().BuildForConsumer(gomock.Any()).Return(nil).AnyTimes()
		mockBuilder.EXPECT().GetAllowedConsumers().Return(nil).AnyTimes()

		return mockBuilder
	}

	GetMockClientFor = func(gwCfg kongutil.GatewayAdminConfig) *kong_clientmock.MockKongClient {
		client, err := kongutil.GetClientFor(gwCfg)
		Expect(err).ToNot(HaveOccurred())
		c, ok := client.(*kong_clientmock.MockKongClient)
		if !ok {
			Fail("unexpected kong-client type")
		}
		return c
	}

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func CreateNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
}
