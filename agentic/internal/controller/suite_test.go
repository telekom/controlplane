// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	agenticconfig "github.com/telekom/controlplane/agentic/internal/config"
)

const (
	timeout  = 30 * time.Second
	interval = 100 * time.Millisecond
)

var (
	ctx         context.Context
	cancel      context.CancelFunc
	k8sClient   client.Client
	testEnv     *envtest.Environment
	managerDone chan error
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Agentic Controller Envtest Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.Background())
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "admin", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "application", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "approval", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "gateway", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "identity", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	scheme := runtime.NewScheme()
	RegisterSchemesOrDie(scheme)
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, Metrics: server.Options{BindAddress: "0"}, HealthProbeBindAddress: "0"})
	Expect(err).NotTo(HaveOccurred())
	Expect(SetupFieldIndexes(ctx, mgr)).To(Succeed())
	Expect((&AgenticExposureReconciler{
		Client: mgr.GetClient(), Scheme: scheme, Config: &agenticconfig.AgenticConfig{},
	}).SetupWithManager(mgr)).To(Succeed())
	Expect((&AgenticSubscriptionReconciler{Client: mgr.GetClient(), Scheme: scheme}).SetupWithManager(mgr)).To(Succeed())
	managerDone = make(chan error, 1)
	go func() { managerDone <- mgr.Start(ctx) }()
	syncCtx, syncCancel := context.WithTimeout(ctx, timeout)
	defer syncCancel()
	Expect(mgr.GetCache().WaitForCacheSync(syncCtx)).To(BeTrue())
})

var _ = AfterSuite(func() {
	defer func() {
		if testEnv != nil {
			Expect(testEnv.Stop()).To(Succeed())
		}
	}()
	if cancel != nil {
		cancel()
	}
	if managerDone != nil {
		Eventually(managerDone, timeout, interval).Should(Receive(Succeed()))
	}
})
