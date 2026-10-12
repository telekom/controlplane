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
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	ptls "github.com/telekom/controlplane/common-server/pkg/client"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/providerca"
	testmock "github.com/telekom/controlplane/common/pkg/test/mock"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	kongclient "github.com/telekom/controlplane/gateway/pkg/kong/client"
	kongmock "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	corev1 "k8s.io/api/core/v1"
)

const (
	timeout         = 5 * time.Second
	interval        = 100 * time.Millisecond
	testNamespace   = "default"
	testEnvironment = "test"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment

	ctx    context.Context
	cancel context.CancelFunc

	mockKongClient *kongmock.MockKongClient
	trustFactory   *kongutil.Factory
	apiReader      client.Reader
	managerCache   cache.Cache

	// trustClockOffset shifts the CA cache clock so tests can expire cached trust.
	trustClockOffset atomic.Int64
)

// expireTrust expires all cached CA resolutions; call it before changing trust inputs
// that tests expect controllers to observe.
func expireTrust() {
	trustClockOffset.Add(int64(2 * time.Minute))
}

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Controller Suite")
}

var _ = BeforeSuite(func() {
	var err error
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("%s-%s-%s", os.Getenv("ENVTEST_K8S_VERSION"), runtime.GOOS, runtime.GOARCH)),
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = gatewayv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
		// Same ConfigMap label filter as cmd/main.go.
		Cache: cclient.ProviderCACacheOptions(nil),
	})
	Expect(err).ToNot(HaveOccurred())

	// Register field indices needed by controllers
	RegisterIndecesOrDie(ctx, k8sManager)

	// Setup MockKongClient with capturing expectations
	mockKongClient = kongmock.NewMockKongClient(GinkgoT())
	setupKongMockExpectations()

	// Real trust resolution through the manager's informer cache, as in cmd/main.go; Kong
	// calls go to the mock.
	caCache := providerca.NewCache(k8sManager.GetClient(), ptls.ParseProviderCABundle,
		providerca.WithClock(func() time.Time { return time.Now().Add(time.Duration(trustClockOffset.Load())) }))
	kongClients := kongutil.NewClientCache(caCache)
	Expect(k8sManager.Add(kongClients)).To(Succeed())
	trustFactory = kongutil.NewFactory(caCache, kongClients, nil)
	kongFactory := kongutil.ClientFactoryFunc(func(ctx context.Context, gw *gatewayv1.Gateway) (kongclient.KongClient, error) {
		if validateErr := trustFactory.Validate(ctx, gw); validateErr != nil {
			return nil, validateErr
		}
		return mockKongClient, nil
	})
	apiReader = k8sManager.GetAPIReader()
	managerCache = k8sManager.GetCache()

	// Override secrets.Get to be an identity function (no real secret resolution needed)
	originalSecretsGet := secretsapi.Get
	secretsapi.Get = func(_ context.Context, secretRef string) (string, error) {
		return secretRef, nil
	}

	// Setup controllers
	err = (&GatewayReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &testmock.EventRecorder{},
		Trust:    trustFactory,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&RouteReconciler{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		Recorder:      &testmock.EventRecorder{},
		ClientFactory: kongFactory,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ConsumerReconciler{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		Recorder:      &testmock.EventRecorder{},
		ClientFactory: kongFactory,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ConsumeRouteReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &testmock.EventRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	// Start manager
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Restore original functions after suite completes
	DeferCleanup(func() {
		secretsapi.Get = originalSecretsGet
	})
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// setupKongMockExpectations configures the MockKongClient to accept all calls
// and allow reconciliation loops to proceed without error.
func setupKongMockExpectations() {
	mockKongClient.On("CreateOrReplaceRoute", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockKongClient.On("CreateOrReplacePlugin", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mockKongClient.On("CleanupPlugins", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockKongClient.On("DeleteRoute", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockKongClient.On("CreateOrReplaceConsumer", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mockKongClient.On("DeleteConsumer", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockKongClient.On("DeleteUpstream", mock.Anything, mock.Anything).Return(nil).Maybe()
}

// createNamespace creates a namespace if it does not already exist.
func createNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := k8sClient.Create(ctx, ns)
	if !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

// newGateway creates a Gateway resource with the given name in the specified namespace.
func newGateway(name, namespace string) *gatewayv1.Gateway {
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: gatewayv1.GatewaySpec{
			Admin: gatewayv1.AdminConfig{
				Url:          "http://kong-admin:8001",
				ClientId:     "test-client-id",
				ClientSecret: "test-client-secret",
				IssuerUrl:    "http://issuer:8080/realms/test",
			},
			Redis: &gatewayv1.RedisConfig{
				Host:     "redis:6379",
				Port:     6379,
				Password: "redis-password",
			},
		},
	}
}
