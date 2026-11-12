// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/internal/api"
	"github.com/telekom/controlplane/secret-manager/internal/handler"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/kubernetes"
	"github.com/telekom/controlplane/secret-manager/pkg/controller"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var _ = Describe("Environment secret initialization", Ordered, func() {
	var kubeClient client.Client
	var app *fiber.App
	var namespace string
	ctx := context.Background()

	BeforeAll(func() {
		testEnv := &envtest.Environment{}
		DeferCleanup(func() {
			Expect(testEnv.Stop()).To(Succeed())
		})
		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		kubeClient, err = client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())

		ctrl := controller.NewController(kubernetes.NewBackend(kubeClient), kubernetes.NewOnboarder(kubeClient))
		app = fiber.New(fiber.Config{ErrorHandler: handler.ErrorHandler})
		api.RegisterHandlers(app, api.NewStrictHandler(handler.NewHandler(ctrl), nil))
	})

	BeforeEach(func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "secret-test-"}}
		Expect(kubeClient.Create(ctx, ns)).To(Succeed())
		namespace = ns.Name
	})

	expectNotFound := func(id kubernetes.Id) {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/secrets/"+url.PathEscape(id.String()), nil))
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(response.Body.Close()).To(Succeed()) }()
		Expect(response.StatusCode).To(Equal(http.StatusNotFound))
		var problem api.ErrorResponse
		Expect(json.NewDecoder(response.Body).Decode(&problem)).To(Succeed())
		Expect(problem.Status).To(Equal(http.StatusNotFound))
		Expect(problem.Title).To(Equal("Not Found"))
		Expect(problem.Detail).To(Equal("resource " + id.String() + " not found"))
	}

	It("returns HTTP 404 before the environment namespace exists", func() {
		id := kubernetes.New("uninitialized", "", "", "zones/zone-a/admin/rover/clientSecret", "")
		expectNotFound(id)
	})

	It("returns HTTP 404 before an environment storage Secret exists", func() {
		id := kubernetes.New(namespace, "", "", "zones/zone-a/admin/rover/clientSecret", "")
		expectNotFound(id)
	})

	It("allows the shared rover credential to be seeded after HTTP 404 and preserves it on retry", func() {
		Expect(kubeClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}})).To(Succeed())
		path := "zones/zone-a/admin/rover/clientSecret"
		id := kubernetes.New("shared", "", "", path, "")
		expectNotFound(id)

		onboarder := kubernetes.NewOnboarder(kubeClient)
		for range 2 {
			_, err := onboarder.OnboardEnvironment(ctx, "shared",
				backend.WithStrategy(backend.StrategyMerge), backend.WithSecretValue(path, backend.String("existing-credential")))
			Expect(err).NotTo(HaveOccurred())
			response, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/secrets/"+url.PathEscape(id.String()), nil))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			var secret api.SecretResponseJSONResponse
			Expect(json.NewDecoder(response.Body).Decode(&secret)).To(Succeed())
			Expect(response.Body.Close()).To(Succeed())
			Expect(secret.Value).To(Equal("existing-credential"))
		}

		expectNotFound(kubernetes.New("shared", "", "", "zones/other-zone/admin/rover/clientSecret", ""))
	})
})
