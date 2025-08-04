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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/test/mock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	secretsapifake "github.com/telekom/controlplane/secret-manager/api/fake"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	timeout         = 2 * time.Second
	interval        = 100 * time.Millisecond
	testNamespace   = "default"
	testEnvironment = "test"
	teamName        = "hyperion"
	group           = "eni"
	teamNamespace   = testEnvironment + "--" + group + "--" + teamName
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment

	ctx    context.Context
	cancel context.CancelFunc

	secretManagerMock *secretsapifake.MockSecretManager
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	var err error
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: append(
			make([]string, 0),
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "api", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "application", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "organization", "config", "crd", "bases"),
		),
		//CRDDirectoryPaths: append(
		//	testutil.GetCrdPathsOrDie("github.com/telekom/controlplane/(api|application|organization)/api"),
		//	filepath.Join("..", "..", "config", "crd", "bases"),
		//),
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("%s-%s-%s", os.Getenv("ENVTEST_K8S_VERSION"), runtime.GOOS, runtime.GOARCH)),
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = roverv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = apiapi.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = applicationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = organizationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	Expect(err).ToNot(HaveOccurred())

	RegisterIndicesOrDie(ctx, k8sManager)

	err = (&ApiSpecificationReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)

	Expect(err).ToNot(HaveOccurred())

	err = (&RoverReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	// Mock SecretManager
	secretManagerMock = secretsapifake.NewMockSecretManager(GinkgoT())
	secretsapi.API = func() secretsapi.SecretManager {
		return secretManagerMock
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

func createNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
}
