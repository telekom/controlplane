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

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/remoteapisubscription/syncer"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test/mock"
	organizationapi "github.com/telekom/controlplane/organization/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	timeout         = 2 * time.Second
	interval        = 100 * time.Millisecond
	testEnvironment = "test"
	testGroup       = "dev"
	testTeamName    = "api"
	testCategory    = "Customer"
)

var testNamespace string

func init() {
	// testNamespace is constructed using the convention: <environment>--<group>--<team>
	testNamespace = testEnvironment + "--" + testGroup + "--" + testTeamName
}

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

var syncerFactoryMock = syncer.NewSyncerFactoryMock()

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(false)))

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: append(
			make([]string, 0),
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "application", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "approval", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "admin", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "gateway", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "identity", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "organization", "config", "crd", "bases"),
		),
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("%s-%s-%s", os.Getenv("ENVTEST_K8S_VERSION"), runtime.GOOS, runtime.GOARCH)),
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	RegisterSchemesOrDie(scheme.Scheme)
	err = apiv1.AddToScheme(scheme.Scheme)
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
	err = (&ApiReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ApiExposureReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ApiSubscriptionReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	remoteApiSubRec := &RemoteApiSubscriptionReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}
	err = remoteApiSubRec.SetupWithManager(k8sManager, syncerFactoryMock)
	Expect(err).ToNot(HaveOccurred())

	apiCategoryRec := &ApiCategoryReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: &mock.EventRecorder{},
	}
	err = apiCategoryRec.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("Creating the environment namespace")
	CreateNamespace(testEnvironment)

	By("Creating the test group and team")
	CreateTestGroup()
	CreateTestTeam()

	By("Creating the test namespace")
	CreateNamespace(testNamespace)

	By("Creating the test API category")
	CreateTestApiCategory()

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

func CreateTestGroup() *organizationapi.Group {
	group := &organizationapi.Group{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testGroup,
			Namespace: testEnvironment,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: organizationapi.GroupSpec{
			DisplayName: "Test Group",
			Description: "Test group for API tests",
		},
	}
	Expect(k8sClient.Create(ctx, group)).To(Succeed())
	return group
}

func CreateTestTeam() *organizationapi.Team {
	team := &organizationapi.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      organizationapi.TeamResourceName(testGroup, testTeamName),
			Namespace: testEnvironment,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: organizationapi.TeamSpec{
			Name:     testTeamName,
			Group:    testGroup,
			Email:    "test-team@example.com",
			Category: organizationapi.TeamCategory(testCategory),
			Members: []organizationapi.Member{
				{
					Name:  "Test User",
					Email: "test@example.com",
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, team)).To(Succeed())
	return team
}

func CreateTestApiCategory() *apiv1.ApiCategory {
	apiCat := &apiv1.ApiCategory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other",
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: apiv1.ApiCategorySpec{
			LabelValue:  "other",
			Active:      true,
			Description: "Other category for testing",
			AllowTeams: &apiv1.AllowTeamsConfig{
				Categories: []string{string(organizationapi.TeamCategoryCustomer)},
				Names:      []string{},
			},
			MustHaveGroupPrefix: false,
		},
	}
	Expect(k8sClient.Create(ctx, apiCat)).To(Succeed())
	return apiCat
}
