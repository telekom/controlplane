// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

const (
	testEnvironment = "test"
	testNamespace   = "default"
)

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	scheme    *k8sruntime.Scheme
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestZoneHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Zone Handler Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "..", "gateway", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "..", "identity", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: filepath.Join("..", "..", "..", "bin", "k8s",
			fmt.Sprintf("%s-%s-%s", os.Getenv("ENVTEST_K8S_VERSION"), runtime.GOOS, runtime.GOARCH)),
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	scheme = k8sruntime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(adminv1.AddToScheme(scheme))
	utilruntime.Must(identityv1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.AddToScheme(scheme))

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Creating test namespace and environment")
	createTestNamespace(testEnvironment)
	createTestEnvironment()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// createTestNamespace creates a namespace for the test environment.
func createTestNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
}

// createTestEnvironment creates the test Environment CR.
func createTestEnvironment() {
	env := &adminv1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testEnvironment,
			Namespace: testEnvironment,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: adminv1.EnvironmentSpec{},
	}
	Expect(k8sClient.Create(ctx, env)).To(Succeed())
}

// newTestZone creates a fully populated zone fixture.
func newTestZone(name string) *adminv1.Zone {
	gatewayAdminSecret := "test-gateway-admin-secret"
	identityAdminUrl := "https://test-iris.de/auth/admin/realms"

	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: adminv1.ZoneSpec{
			IdentityProvider: adminv1.IdentityProviderConfig{
				Admin: adminv1.IdentityProviderAdminConfig{
					Url:      &identityAdminUrl,
					ClientId: "test-idp-admin-id",
					UserName: "test-idp-admin-username",
					Password: "test-idp-admin-password",
				},
				Url: "https://test-iris.de/",
			},
			Gateway: adminv1.GatewayConfig{
				Admin: adminv1.GatewayAdminConfig{
					ClientSecret: &gatewayAdminSecret,
					Url:          "https://test-stargate.de/admin-api",
				},
				Url: "https://test-stargate.de/",
			},
			Redis: adminv1.RedisConfig{
				Host:      "http://test-redis.de/",
				Port:      123,
				Password:  "test-redis-password",
				EnableTLS: true,
			},
			Visibility: adminv1.ZoneVisibilityWorld,
		},
	}
}

// newTestContext builds a context with the JanitorClient and environment injected,
// ready for handler step functions.
func newTestContext(zone *adminv1.Zone) context.Context {
	envName := zone.Labels[config.EnvironmentLabelKey]
	scopedClient := cclient.NewScopedClient(k8sClient, envName)
	janitor := cclient.NewJanitorClient(scopedClient)
	testCtx := contextutil.WithEnv(ctx, envName)
	testCtx = cclient.WithClient(testCtx, janitor)
	return testCtx
}

// newTestHandlingContext creates a HandlingContext by running the constructor
// (which creates the namespace and fetches the environment).
func newTestHandlingContext(testCtx context.Context, zone *adminv1.Zone) *HandlingContext {
	hc, err := newHandlingContext(testCtx, zone)
	Expect(err).NotTo(HaveOccurred())
	return hc
}
