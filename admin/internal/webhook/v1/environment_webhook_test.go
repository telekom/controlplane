// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Environment Webhook", func() {
	var (
		obj       *adminv1.Environment
		oldObj    *adminv1.Environment
		validator EnvironmentCustomValidator
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &adminv1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-env",
				Namespace: "default",
			},
			Spec: adminv1.EnvironmentSpec{
				RealmName: "my-realm",
			},
		}
		oldObj = obj.DeepCopy()
		validator = EnvironmentCustomValidator{Client: k8sClient}
	})

	Context("When creating Environment under Validating Webhook", func() {
		It("Should admit creation with a unique realm name", func() {
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("When updating Environment under Validating Webhook", func() {
		It("Should deny update if realmName is changed after being set", func() {
			oldObj.Spec.RealmName = "original"
			obj.Spec.RealmName = "changed"
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("immutable"))
		})

		It("Should admit update if realmName is unchanged", func() {
			oldObj.Spec.RealmName = "same"
			obj.Spec.RealmName = "same"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should admit setting realmName for the first time", func() {
			oldObj.Spec.RealmName = ""
			obj.Spec.RealmName = "new-realm"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("When deleting Environment under Validating Webhook", func() {
		It("Should always admit deletion", func() {
			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})
})
