// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Roadmap V1 Test Suite", func() {
	Context("Roadmap Types", func() {
		It("should accept a valid Roadmap with API resource type", func() {
			roadmap := new(v1.Roadmap)
			roadmap.Name = "test-roadmap-api"
			roadmap.Namespace = "default"
			roadmap.Spec = v1.RoadmapSpec{
				ResourceName: "/eni/my-api/v1",
				ResourceType: v1.ResourceTypeAPI,
				Roadmap:      "test--eni--team--my-api-v1",
				Hash:         "abc123hash",
			}
			roadmap.Status = v1.RoadmapStatus{}

			err := k8sClient.Create(ctx, roadmap)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Delete(ctx, roadmap)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept a valid Roadmap with Event resource type", func() {
			roadmap := new(v1.Roadmap)
			roadmap.Name = "test-roadmap-event"
			roadmap.Namespace = "default"
			roadmap.Spec = v1.RoadmapSpec{
				ResourceName: "de.telekom.eni.myevent.v1",
				ResourceType: v1.ResourceTypeEvent,
				Roadmap:      "test--eni--team--myevent",
				Hash:         "def456hash",
			}
			roadmap.Status = v1.RoadmapStatus{}

			err := k8sClient.Create(ctx, roadmap)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Delete(ctx, roadmap)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject a Roadmap with empty resourceName", func() {
			roadmap := new(v1.Roadmap)
			roadmap.Name = "invalid-roadmap-no-name"
			roadmap.Namespace = "default"
			roadmap.Spec = v1.RoadmapSpec{
				ResourceName: "", // Empty resourceName
				ResourceType: v1.ResourceTypeAPI,
				Roadmap:      "test--file-id",
				Hash:         "somehash",
			}
			roadmap.Status = v1.RoadmapStatus{}

			err := k8sClient.Create(ctx, roadmap)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			Expect(statusErr.Status().Details.Causes).To(ContainElement(metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: "Invalid value: \"\": spec.resourceName in body should be at least 1 chars long",
				Field:   "spec.resourceName",
			}))
		})

		It("should reject a Roadmap with invalid resourceType", func() {
			roadmap := new(v1.Roadmap)
			roadmap.Name = "invalid-roadmap-type"
			roadmap.Namespace = "default"
			roadmap.Spec = v1.RoadmapSpec{
				ResourceName: "/eni/my-api/v1",
				ResourceType: "InvalidType", // Invalid enum value
				Roadmap:      "test--file-id",
				Hash:         "somehash",
			}
			roadmap.Status = v1.RoadmapStatus{}

			err := k8sClient.Create(ctx, roadmap)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			Expect(statusErr.Status().Details.Causes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(metav1.CauseTypeFieldValueNotSupported),
				"Field": Equal("spec.resourceType"),
			})))
		})

		It("should reject a Roadmap with empty roadmap file ID", func() {
			roadmap := new(v1.Roadmap)
			roadmap.Name = "invalid-roadmap-no-file"
			roadmap.Namespace = "default"
			roadmap.Spec = v1.RoadmapSpec{
				ResourceName: "/eni/my-api/v1",
				ResourceType: v1.ResourceTypeAPI,
				Roadmap:      "", // Empty file ID
				Hash:         "somehash",
			}
			roadmap.Status = v1.RoadmapStatus{}

			err := k8sClient.Create(ctx, roadmap)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			// The error message might vary, but field should be spec.roadmap
			Expect(statusErr.Status().Details.Causes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Field": Equal("spec.roadmap"),
			})))
		})

		It("should reject a Roadmap with empty hash", func() {
			roadmap := new(v1.Roadmap)
			roadmap.Name = "invalid-roadmap-no-hash"
			roadmap.Namespace = "default"
			roadmap.Spec = v1.RoadmapSpec{
				ResourceName: "/eni/my-api/v1",
				ResourceType: v1.ResourceTypeAPI,
				Roadmap:      "test--file-id",
				Hash:         "", // Empty hash
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
