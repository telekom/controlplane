// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
// Copyright 2026.
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx          context.Context
	cancel       context.CancelFunc
	testEnv      *envtest.Environment
	cfg          *rest.Config
	k8sClient    client.Client
	directClient client.Client // uncached client for test assertions
	testScheme   *runtime.Scheme
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	testScheme = runtime.NewScheme()
	RegisterSchemesOrDie(testScheme)

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "gateway", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "pubsub", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "approval", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "application", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "admin", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "event", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "identity", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
		Scheme:                testScheme,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Use a manager to get a cached client with field indexes.
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: testScheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics server in tests
		},
	})
	Expect(err).NotTo(HaveOccurred())

	// Register field index for EventConfig zone lookup.
	err = mgr.GetFieldIndexer().IndexField(ctx, &eventv1.EventConfig{}, ".spec.zone.name",
		func(obj client.Object) []string {
			ec, ok := obj.(*eventv1.EventConfig)
			if !ok || ec.Spec.Zone.Name == "" {
				return nil
			}
			return []string{ec.Spec.Zone.Name}
		})
	Expect(err).NotTo(HaveOccurred())

	// Register owner indexes required by JanitorClient.Cleanup.
	Expect(index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &approvalv1.ApprovalRequest{})).To(Succeed())
	Expect(index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &pubsubv1.Subscriber{})).To(Succeed())
	Expect(index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &pubsubv1.Publisher{})).To(Succeed())
	Expect(index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &gatewayv1.Route{})).To(Succeed())
	Expect(index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &gatewayv1.RouteListener{})).To(Succeed())

	// Start the manager cache in a goroutine.
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).To(Succeed())
	}()

	// Wait for the cache to sync.
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())

	// Use the manager's cached client for reconcilers (supports MatchingFields).
	k8sClient = mgr.GetClient()
	Expect(k8sClient).NotTo(BeNil())

	// Create a direct (uncached) client for test assertions and CR creation.
	directClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	Eventually(func() error {
		return testEnv.Stop()
	}, time.Minute, time.Second).Should(Succeed())
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
