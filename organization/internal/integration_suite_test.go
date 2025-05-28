// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/test/mock"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/organization/internal/controller"
	"github.com/telekom/controlplane/organization/internal/index"
	"github.com/telekom/controlplane/organization/internal/secret"
	secretmock "github.com/telekom/controlplane/organization/internal/secret/mock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	webhhookv1 "github.com/telekom/controlplane/organization/internal/webhook/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: append(
			[]string{
				filepath.Join("..", "..", "admin", "config", "crd", "bases"),
				filepath.Join("..", "..", "gateway", "config", "crd", "bases"),
				filepath.Join("..", "..", "identity", "config", "crd", "bases"),
			},
			filepath.Join("..", "config", "crd", "bases")),
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: filepath.Join("..", "bin", "k8s",
			fmt.Sprintf("%s-%s-%s", os.Getenv("ENVTEST_K8S_VERSION"), runtime.GOOS, runtime.GOARCH)),
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "config", "webhook")}},
	}

	var err error

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = organizationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = adminv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = gatewayv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = identityv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("setting up the manager")
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
		}),
		LeaderElection: false,
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	Expect(err).ToNot(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Registering all required indices")
	index.RegisterIndicesOrDie(ctx, k8sManager)

	By("Setting up controllers & reconcilers")
	err = (&controller.GroupReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)

	Expect(err).ToNot(HaveOccurred())

	err = (&controller.TeamReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("setting up the team webhook")
	err = webhhookv1.SetupTeamWebhookWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	By("setting up the mocked secret manager for the webhook")
	secret.GetSecretManager = secretmock.SecretManager

	By("Creating the environment namespace")
	CreateNamespace(testEnvironment)

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

func ExpectObjConditionToBeReady(g Gomega, obj types.Object) {
	g.Expect(obj.GetConditions()).To(HaveLen(2))
	readyCondition := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
	g.Expect(readyCondition).NotTo(BeNil())
	g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
}
