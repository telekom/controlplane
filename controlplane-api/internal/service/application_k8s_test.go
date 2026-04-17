// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ApplicationK8sService", func() {
	var (
		svc       service.ApplicationService
		k8sClient client.Client
	)

	seedApp := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "dev--team-alpha",
			Labels: map[string]string{
				config.EnvironmentLabelKey: "dev",
			},
		},
		Spec: applicationv1.ApplicationSpec{
			Team:      "team-alpha",
			TeamEmail: "alpha@example.com",
			Secret:    "$<some-secret-ref>",
		},
	}

	rotateInput := model.RotateApplicationSecretInput{
		Environment: "dev",
		Team:        "team-alpha",
		Name:        "my-app",
	}

	BeforeEach(func() {
		k8sClient = newFakeClient(seedApp.DeepCopy())
		svc = service.NewApplicationK8sService(k8sClient)
	})

	Describe("RotateApplicationSecret", func() {
		Describe("Authorization", func() {
			It("should allow admin to rotate any application secret", func() {
				result, err := svc.RotateApplicationSecret(adminCtx(), rotateInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should allow team viewer owning the application", func() {
				result, err := svc.RotateApplicationSecret(teamCtx("team-alpha"), rotateInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
			})

			It("should deny team viewer not owning the application", func() {
				_, err := svc.RotateApplicationSecret(teamCtx("team-beta"), rotateInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("forbidden"))
			})

			It("should deny when no viewer is present", func() {
				_, err := svc.RotateApplicationSecret(noViewerCtx(), rotateInput)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unauthorized"))
			})
		})

		Describe("Success", func() {
			It("should set Spec.Secret to 'rotate' and return correct result", func() {
				result, err := svc.RotateApplicationSecret(adminCtx(), rotateInput)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeTrue())
				Expect(result.Message).To(Equal("application secret rotation initiated"))
				Expect(*result.Namespace).To(Equal("dev--team-alpha"))
				Expect(*result.ResourceName).To(Equal("my-app"))

				// Verify Spec.Secret was set on the CRD
				app := &applicationv1.Application{}
				err = k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: "dev--team-alpha",
					Name:      "my-app",
				}, app)
				Expect(err).NotTo(HaveOccurred())
				Expect(app.Spec.Secret).To(Equal("rotate"))
			})
		})

		Describe("Not found", func() {
			It("should return error when application does not exist", func() {
				_, err := svc.RotateApplicationSecret(adminCtx(), model.RotateApplicationSecretInput{
					Environment: "dev",
					Name:        "nonexistent-app",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})
	})
})
