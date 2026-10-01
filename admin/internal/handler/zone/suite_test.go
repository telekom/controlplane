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
	"slices"
	"strings"
	"sync"
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
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test/mock"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
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

// newTestZone creates a fully populated zone fixture using the Presets-based GatewayConfig.
func newTestZone(name string) *adminv1.Zone {
	identityAdminURL := "https://test-iris.de/auth/admin/realms"

	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: adminv1.ZoneSpec{
			IdentityProviders: []adminv1.IdentityProviderConfig{{
				Name: "primary",
				Admin: adminv1.IdentityProviderAdminConfig{
					Url:      &identityAdminURL,
					ClientId: "test-idp-admin-id",
					UserName: "test-idp-admin-username",
					Password: "test-idp-admin-password",
				},
				IssuerHostname: "test-iris.de",
				TokenUrl:       "https://test-iris.de/auth/realms/test/protocol/openid-connect/token",
			}},
			Gateways: []adminv1.GatewayConfig{{
				Name: "standard",
				Admin: adminv1.GatewayAdminConfig{
					Url:                 "https://test-stargate.de/admin-api",
					IdentityProviderRef: "primary",
				},
			}},
			Presets: []adminv1.Preset{{
				Name: "default", Type: adminv1.GatewayTypeAPI, Default: true, GatewayRef: "standard", IdentityProviderRef: "primary",
				Urls: []adminv1.UrlConfig{{Hostname: "test-stargate.de", BasePath: "/"}},
			}},
			Redis: &adminv1.RedisConfig{
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
	return newTestContextWithClient(zone, k8sClient)
}

// newTestContextWithClient is newTestContext with a custom underlying Kubernetes client.
func newTestContextWithClient(zone *adminv1.Zone, c client.Client) context.Context {
	envName := zone.Labels[config.EnvironmentLabelKey]
	scopedClient := cclient.NewScopedClient(c, envName)
	janitor := cclient.NewJanitorClient(scopedClient)
	testCtx := contextutil.WithEnv(ctx, envName)
	testCtx = cclient.WithClient(testCtx, janitor)
	testCtx = contextutil.WithRecorder(testCtx, &mock.EventRecorder{})
	return testCtx
}

// newTestHandler installs a fresh in-memory secret-manager for the current spec and
// returns a ZoneHandler.
func newTestHandler() *ZoneHandler {
	useSecretManager()
	return &ZoneHandler{}
}

// useSecretManager installs a fresh in-memory secret-manager as the global secret-manager
// API for the current spec.
func useSecretManager() *memorySecretManager {
	sm := newMemorySecretManager()
	original := secretsapi.API
	secretsapi.API = func() secretsapi.SecretManager { return sm }
	DeferCleanup(func() { secretsapi.API = original })
	return sm
}

// memorySecretManager is an in-memory secret-manager storing environment secrets by path.
type memorySecretManager struct {
	mu        sync.Mutex
	values    map[string]string
	published []string
	version   int
	failNext  int
	getErr    error
}

func newMemorySecretManager() *memorySecretManager {
	return &memorySecretManager{values: map[string]string{}}
}

// memoryKey maps a secret ID (env:team:app:path:checksum) to its storage key, ignoring the checksum.
func memoryKey(ref string) string {
	id, _ := secretsapi.FromRef(ref)
	parts := strings.Split(id, ":")
	Expect(parts).To(HaveLen(5), "invalid secret id %q", id)
	return parts[0] + ":" + parts[3]
}

func (m *memorySecretManager) UpsertEnvironment(_ context.Context, envID string, opts ...secretsapi.OnboardingOption) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	options := &secretsapi.OnboardingOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if m.failNext > 0 {
		m.failNext--
		return nil, fmt.Errorf("secret-manager unavailable")
	}
	available := map[string]string{}
	for path, value := range options.SecretValues {
		str, ok := value.(string)
		Expect(ok).To(BeTrue())
		m.version++
		m.values[envID+":"+path] = str
		m.published = append(m.published, str)
		available[path] = fmt.Sprintf("%s:::%s:%d", envID, path, m.version)
	}
	return available, nil
}

func (m *memorySecretManager) Get(_ context.Context, ref string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return "", m.getErr
	}
	value, ok := m.values[memoryKey(ref)]
	if !ok {
		return "", secretsapi.ErrNotFound
	}
	return value, nil
}

func (m *memorySecretManager) valueOf(ref string) string {
	GinkgoHelper()
	m.mu.Lock()
	defer m.mu.Unlock()
	Expect(secretsapi.IsRef(ref)).To(BeTrue(), "expected a secret-manager reference")
	return m.values[memoryKey(ref)]
}

func (m *memorySecretManager) publishedValues() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.published)
}

func (m *memorySecretManager) Set(context.Context, string, string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (m *memorySecretManager) Rotate(context.Context, string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (m *memorySecretManager) UpsertTeam(context.Context, string, string, ...secretsapi.OnboardingOption) (map[string]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *memorySecretManager) UpsertApplication(context.Context, string, string, string, ...secretsapi.OnboardingOption) (map[string]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *memorySecretManager) DeleteEnvironment(context.Context, string) error {
	return fmt.Errorf("not implemented")
}

func (m *memorySecretManager) DeleteTeam(context.Context, string, string) error {
	return fmt.Errorf("not implemented")
}

func (m *memorySecretManager) DeleteApplication(context.Context, string, string, string) error {
	return fmt.Errorf("not implemented")
}

// newTestHandlingContext creates a HandlingContext by running the constructor
// (which creates the namespace and fetches the environment).
func newTestHandlingContext(testCtx context.Context, zone *adminv1.Zone) *HandlingContext {
	hc, err := newHandlingContext(testCtx, zone, (&ZoneHandler{}).httpClient())
	Expect(err).NotTo(HaveOccurred())
	return hc
}

// markSubResourcesReady stands in for the identity operator: it marks the
// resources the pipeline barrier waits for as provisioned.
func markSubResourcesReady(zone *adminv1.Zone) {
	GinkgoHelper()

	idp := &identityv1.IdentityProvider{}
	Expect(zone.Status.IdentityProvider).NotTo(BeNil())
	Expect(k8sClient.Get(ctx, zone.Status.IdentityProvider.K8s(), idp)).To(Succeed())
	idp.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "IdentityProvider is ready"))
	Expect(k8sClient.Status().Update(ctx, idp)).To(Succeed())

	for _, ref := range []*types.ObjectRef{zone.Status.IdentityRealm, zone.Status.InternalIdentityRealm} {
		Expect(ref).NotTo(BeNil())
		realm := &identityv1.Realm{}
		Expect(k8sClient.Get(ctx, ref.K8s(), realm)).To(Succeed())
		realm.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Realm is ready"))
		Expect(k8sClient.Status().Update(ctx, realm)).To(Succeed())
	}
}
