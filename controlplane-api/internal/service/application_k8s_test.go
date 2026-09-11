// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApplicationK8sService", func() {
	var (
		svc       service.ApplicationService
		k8sClient client.Client
	)

	app := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "dev--team-alpha",
			Labels: map[string]string{
				config.EnvironmentLabelKey: "poc",
			},
		},
		Spec: applicationv1.ApplicationSpec{
			Team:      "team-alpha",
			TeamEmail: "alpha@example.com",
			Secret:    "$<some-secret-ref>",
		},
	}

	BeforeEach(func() {
		k8sClient = newFakeClient(app.DeepCopy())
		svc = service.NewApplicationK8sService(cc.NewScopedClient(k8sClient, "poc"))
	})

	Describe("RotateApplicationSecret", func() {
		It("should set the secret to rotate keyword", func() {
			ref := service.ResourceRef{
				Namespace: "dev--team-alpha",
				Name:      "my-app",
				TeamName:  "team-alpha",
			}
			payload, err := svc.RotateApplicationSecret(adminCtx(), ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Accepted).To(BeTrue())
			Expect(payload.Errors).To(BeEmpty())
		})

		It("should return error when application not found", func() {
			ref := service.ResourceRef{
				Namespace: "dev--team-alpha",
				Name:      "nonexistent",
				TeamName:  "team-alpha",
			}
			payload, err := svc.RotateApplicationSecret(adminCtx(), ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Errors).ToNot(BeEmpty())
		})

		It("should return forbidden when not authorized", func() {
			ref := service.ResourceRef{
				Namespace: "dev--team-alpha",
				Name:      "my-app",
				TeamName:  "other-team",
			}
			payload, err := svc.RotateApplicationSecret(teamCtx("team-alpha"), ref)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Errors).ToNot(BeEmpty())
			Expect(payload.Errors[0].Code).To(Equal(model.ErrorCodeForbidden))
		})
	})
})
