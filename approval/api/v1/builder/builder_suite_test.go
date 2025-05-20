// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	"github.com/telekom/controlplane/common/pkg/test"
	"github.com/telekom/controlplane/common/pkg/test/testutil"
	ctrl "sigs.k8s.io/controller-runtime"
	crscheme "sigs.k8s.io/controller-runtime/pkg/scheme"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	timeout         = 2 * time.Second
	interval        = 300 * time.Millisecond
	testNamespace   = "default"
	testEnvironment = "test"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc
var k8sm manager.Manager

func TestBuilder(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Builder Suite")
}

var _ = BeforeSuite(func() {
	var err error

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	err = (&crscheme.Builder{
		GroupVersion: schema.GroupVersion{
			Group:   "testgroup.cp.ei.telekom.de",
			Version: "v1",
		},
	}).Register(&test.TestResource{}, &test.TestResourceList{}).AddToScheme(scheme.Scheme)

	Expect(err).NotTo(HaveOccurred())

	By("bootstrapping test environment")
	testutil.PkgModCrdsSubpath = "pkg/test/testdata/crds"
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: append(
			testutil.GetCrdPathsOrDie("github.com/telekom/controlplane/common"),
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		),
		ErrorIfCRDPathMissing: true,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "..", "bin", "k8s",
			fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = approvalv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Creating the manager")
	k8sm, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	Expect(err).ToNot(HaveOccurred())

	err = index.SetOwnerIndex(ctx, k8sm.GetFieldIndexer(), &approvalv1.ApprovalRequest{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}
	go func() {
		defer GinkgoRecover()
		err = k8sm.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
