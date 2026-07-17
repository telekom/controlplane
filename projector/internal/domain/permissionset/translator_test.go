// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package permissionset_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/permissionset"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

var _ = Describe("PermissionSet Translator", func() {
	var tr permissionset.Translator

	Describe("ShouldSkip", func() {
		It("should skip when the application label is missing", func() {
			obj := &permissionv1.PermissionSet{}
			skip, reason := tr.ShouldSkip(obj)
			Expect(skip).To(BeTrue())
			Expect(reason).To(ContainSubstring("cp.ei.telekom.de/application"))
		})

		It("should skip when the application label is empty", func() {
			obj := &permissionv1.PermissionSet{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"cp.ei.telekom.de/application": "",
					},
				},
			}
			skip, reason := tr.ShouldSkip(obj)
			Expect(skip).To(BeTrue())
			Expect(reason).To(ContainSubstring("cp.ei.telekom.de/application"))
		})

		It("should not skip when the application label is present", func() {
			obj := &permissionv1.PermissionSet{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"cp.ei.telekom.de/application": "my-app",
					},
				},
			}
			skip, reason := tr.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should populate all fields from the CR", func() {
			obj := &permissionv1.PermissionSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "prod",
						"cp.ei.telekom.de/application": "my-app",
					},
				},
				Spec: permissionv1.PermissionSetSpec{
					Permissions: []permissionv1.Permission{
						{Role: "admin", Resource: "orders", Actions: []string{"read", "write"}},
						{Role: "viewer", Resource: "orders", Actions: []string{"read"}},
					},
				},
				Status: permissionv1.PermissionSetStatus{
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", Message: "all good"},
					},
				},
			}

			data, err := tr.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.Meta.Namespace).To(Equal("prod--platform--narvi"))
			Expect(data.Meta.Name).To(Equal("my-app"))
			Expect(data.StatusPhase).To(Equal("READY"))
			Expect(data.StatusMessage).To(Equal("all good"))
			Expect(data.AppName).To(Equal("my-app"))
			Expect(data.TeamName).To(Equal("platform--narvi"))
			Expect(data.Permissions).To(HaveLen(2))
			Expect(data.Permissions[0].Role).To(Equal("admin"))
			Expect(data.Permissions[0].Resource).To(Equal("orders"))
			Expect(data.Permissions[0].Actions).To(Equal([]string{"read", "write"}))
			Expect(data.Permissions[1].Role).To(Equal("viewer"))
		})

		It("should derive UNKNOWN status when no conditions are set", func() {
			obj := &permissionv1.PermissionSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/application": "my-app",
					},
				},
				Spec: permissionv1.PermissionSetSpec{
					Permissions: []permissionv1.Permission{
						{Role: "admin", Resource: "orders", Actions: []string{"read"}},
					},
				},
			}

			data, err := tr.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.StatusPhase).To(Equal("UNKNOWN"))
		})

		It("should handle an empty permissions list", func() {
			obj := &permissionv1.PermissionSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/application": "my-app",
					},
				},
			}

			data, err := tr.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Permissions).To(BeEmpty())
		})
	})

	Describe("KeyFromObject", func() {
		It("should return the composite key from CR fields", func() {
			obj := &permissionv1.PermissionSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/application": "my-app",
					},
				},
			}
			key := tr.KeyFromObject(obj)
			Expect(key).To(Equal(permissionset.PermissionSetKey{
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}))
		})
	})

	Describe("KeyFromDelete", func() {
		It("should use CR fields from lastKnown when available", func() {
			lastKnown := &permissionv1.PermissionSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/application": "my-app",
					},
				},
			}
			key, err := tr.KeyFromDelete(k8stypes.NamespacedName{Name: "irrelevant", Namespace: "irrelevant"}, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key).To(Equal(permissionset.PermissionSetKey{
				AppName:  "my-app",
				TeamName: "platform--narvi",
			}))
		})

		It("should return ErrDeleteKeyLost when lastKnown is nil", func() {
			key, err := tr.KeyFromDelete(k8stypes.NamespacedName{
				Name:      "my-app",
				Namespace: "prod--platform--narvi",
			}, nil)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, runtime.ErrDeleteKeyLost)).To(BeTrue())
			Expect(key).To(Equal(permissionset.PermissionSetKey{}))
		})
	})
})
