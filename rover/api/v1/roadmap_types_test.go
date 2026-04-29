// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/types"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Roadmap V1 Test Suite", func() {
	Context("Roadmap Types", func() {
		It("should accept a valid Roadmap with specification reference", func() {
			roadmap := new(v1.Roadmap)
			roadmap.Name = "test-roadmap-api"
			roadmap.Namespace = "default"
			roadmap.Spec = v1.RoadmapSpec{
				SpecificationRef: types.TypedObjectRef{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ApiSpecification",
						APIVersion: "rover.cp.ei.telekom.de/v1",
					},
					ObjectRef: types.ObjectRef{
						Name:      "eni-my-api",
						Namespace: "default",
					},
				},
				Contents: "test--eni--team--my-api-v1",
				Hash:     "abc123hash",
			}
			roadmap.Status = v1.RoadmapStatus{}

			err := k8sClient.Create(ctx, roadmap)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Delete(ctx, roadmap)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject a Roadmap with empty specification name", func() {
			roadmap := new(v1.Roadmap)
			roadmap.Name = "invalid-roadmap-no-apispec-name"
			roadmap.Namespace = "default"
			roadmap.Spec = v1.RoadmapSpec{
				SpecificationRef: types.TypedObjectRef{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ApiSpecification",
						APIVersion: "rover.cp.ei.telekom.de/v1",
					},
					ObjectRef: types.ObjectRef{
						Name:      "", // Empty name
						Namespace: "default",
					},
				},
				Contents: "test--file-id",
				Hash:     "somehash",
			}
			roadmap.Status = v1.RoadmapStatus{}

			err := k8sClient.Create(ctx, roadmap)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			Expect(statusErr.Status().Details.Causes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Field": Equal("spec.specificationRef.name"),
			})))
		})

		It("should reject a Roadmap with empty specification namespace", func() {
			roadmap := new(v1.Roadmap)
			roadmap.Name = "invalid-roadmap-no-apispec-ns"
			roadmap.Namespace = "default"
			roadmap.Spec = v1.RoadmapSpec{
				SpecificationRef: types.TypedObjectRef{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ApiSpecification",
						APIVersion: "rover.cp.ei.telekom.de/v1",
					},
					ObjectRef: types.ObjectRef{
						Name:      "eni-my-api",
						Namespace: "", // Empty namespace
					},
				},
				Contents: "test--file-id",
				Hash:     "somehash",
			}
			roadmap.Status = v1.RoadmapStatus{}

			err := k8sClient.Create(ctx, roadmap)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			Expect(statusErr.Status().Details.Causes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Field": Equal("spec.specificationRef.namespace"),
			})))
		})

		It("should reject a Roadmap with empty contents file ID", func() {
			roadmap := new(v1.Roadmap)
			roadmap.Name = "invalid-roadmap-no-file"
			roadmap.Namespace = "default"
			roadmap.Spec = v1.RoadmapSpec{
				SpecificationRef: types.TypedObjectRef{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ApiSpecification",
						APIVersion: "rover.cp.ei.telekom.de/v1",
					},
					ObjectRef: types.ObjectRef{
						Name:      "eni-my-api",
						Namespace: "default",
					},
				},
				Contents: "", // Empty file ID
				Hash:     "somehash",
			}
			roadmap.Status = v1.RoadmapStatus{}

			err := k8sClient.Create(ctx, roadmap)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			// The error message might vary, but field should be spec.contents
			Expect(statusErr.Status().Details.Causes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Field": Equal("spec.contents"),
			})))
		})

		It("should reject a Roadmap with empty hash", func() {
			roadmap := new(v1.Roadmap)
			roadmap.Name = "invalid-roadmap-no-hash"
			roadmap.Namespace = "default"
			roadmap.Spec = v1.RoadmapSpec{
				SpecificationRef: types.TypedObjectRef{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ApiSpecification",
						APIVersion: "rover.cp.ei.telekom.de/v1",
					},
					ObjectRef: types.ObjectRef{
						Name:      "eni-my-api",
						Namespace: "default",
					},
				},
				Contents: "test--file-id",
				Hash:     "", // Empty hash
			}
			roadmap.Status = v1.RoadmapStatus{}

			err := k8sClient.Create(ctx, roadmap)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			// The error message might vary, but field should be spec.hash
			Expect(statusErr.Status().Details.Causes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Field": Equal("spec.hash"),
			})))
		})
	})
})
