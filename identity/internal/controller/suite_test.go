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

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/providerca"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/test/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	timeout         = 2 * time.Second
	interval        = 200 * time.Millisecond
	testNamespace   = "default"
	testEnvironment = "test"
	mockKeycloak    = true
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc

	sharedFactory *keycloak.Factory
	caCache       keycloak.CAResolver
	k8sManager    ctrl.Manager
	realmRec      *RealmReconciler
	clientRec     *ClientReconciler
	idpRec        *IdentityProviderReconciler

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

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("%s-%s-%s", os.Getenv("ENVTEST_K8S_VERSION"), runtime.GOOS, runtime.GOARCH)),
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = identityv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Cache:  cclient.ProviderCACacheOptions(nil),
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	Expect(err).ToNot(HaveOccurred())

	RegisterIndexesOrDie(ctx, k8sManager)

	By("Setting up the required mocks")
	// Wired like cmd/main.go: trust resolves through the manager's informer cache; only the
	// remote Keycloak API is mocked.
	caCache = providerca.NewCache(k8sManager.GetClient(), commonclient.ParseProviderCABundle, providerca.WithClock(func() time.Time {
		return time.Now().Add(time.Duration(trustClockOffset.Load()))
	}))
	clients := keycloak.NewClientCache(caCache)
	Expect(k8sManager.Add(clients)).To(Succeed())
	sharedFactory = keycloak.NewServiceFactory(clients,
		keycloak.WithServiceBuilder(func(_, _ context.Context, _ *keycloak.Connection) (keycloak.KeycloakService, func(), error) {
			if mockKeycloak {
				mockClient := utils.NewKeycloakClientMock(GinkgoT())
				utils.ConfigureKeycloakClientMock(mockClient)
				return keycloak.NewKeycloakService(mockClient), func() {}, nil
			}
			return nil, nil, fmt.Errorf("real keycloak is not available in tests")
		}))
	mockFactory := sharedFactory

	clientRec = &ClientReconciler{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		ClientFactory: mockFactory,
	}
	err = clientRec.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	idpRec = &IdentityProviderReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		CA:     caCache,
	}
	err = idpRec.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	realmRec = &RealmReconciler{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		ClientFactory: mockFactory,
		CA:            caCache,
	}
	err = realmRec.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

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
