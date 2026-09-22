// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package log

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SanitizeForLog", func() {

	newObjectWithManagedFields := func() *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-configmap",
				Namespace: "my-namespace",
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:   "some-controller",
						Operation: metav1.ManagedFieldsOperationUpdate,
					},
				},
			},
			Data: map[string]string{
				"key": "value",
			},
		}
	}

	It("should return a copy without managedFields", func() {
		original := newObjectWithManagedFields()

		sanitized := SanitizeForLog(original)

		Expect(sanitized.GetManagedFields()).To(BeEmpty())
	})

	It("should not mutate the original object", func() {
		original := newObjectWithManagedFields()

		SanitizeForLog(original)

		Expect(original.GetManagedFields()).ToNot(BeEmpty())
		Expect(original.GetManagedFields()).To(HaveLen(1))
	})

	It("should preserve the other fields of the object", func() {
		original := newObjectWithManagedFields()

		sanitized := SanitizeForLog(original)

		Expect(sanitized.GetName()).To(Equal(original.GetName()))
		Expect(sanitized.GetNamespace()).To(Equal(original.GetNamespace()))
		sanitizedCM, ok := sanitized.(*corev1.ConfigMap)
		Expect(ok).To(BeTrue())
		Expect(sanitizedCM.Data).To(Equal(original.Data))
	})

	It("should return nil when given a nil object", func() {
		Expect(SanitizeForLog(nil)).To(BeNil())
	})

	It("should keep managedFields when sanitization is disabled via env var", func() {
		GinkgoT().Setenv(DisableSanitizationEnvVar, "true")
		original := newObjectWithManagedFields()

		sanitized := SanitizeForLog(original)

		Expect(sanitized.GetManagedFields()).To(HaveLen(1))
		Expect(sanitized).To(BeIdenticalTo(original))
	})

	It("should sanitize when the disable env var is unset, empty, or invalid", func() {
		for _, value := range []string{"", "not-a-bool", "false"} {
			GinkgoT().Setenv(DisableSanitizationEnvVar, value)
			original := newObjectWithManagedFields()

			sanitized := SanitizeForLog(original)

			Expect(sanitized.GetManagedFields()).To(BeEmpty(), "value=%q", value)
		}
	})
})
