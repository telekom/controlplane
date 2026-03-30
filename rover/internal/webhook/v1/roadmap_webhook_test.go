// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/common/pkg/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Roadmap Webhook", func() {

	Context("Validating", func() {

		var ctx = context.Background()

		It("should allow valid Roadmap with API resource type", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.RoadmapSpec{
					ResourceName: "/eni/my-api/v1",
					ResourceType: roverv1.ResourceTypeAPI,
					Roadmap:      "test--eni--team--my-api-v1",
					Hash:         "abc123hash",
				},
			}

			validator := &RoadmapCustomValidator{}

			warnings, err := validator.ValidateCreate(ctx, roadmap)

			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("should allow valid Roadmap with Event resource type", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap-event",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.RoadmapSpec{
					ResourceName: "de.telekom.eni.myevent.v1",
					ResourceType: roverv1.ResourceTypeEvent,
					Roadmap:      "test--eni--team--myevent",
					Hash:         "def456hash",
				},
			}

			validator := &RoadmapCustomValidator{}

			warnings, err := validator.ValidateCreate(ctx, roadmap)

			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("should block when the environment label is missing", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap",
					Namespace: "test--my-group--my-team",
					Labels:    map[string]string{},
				},
				Spec: roverv1.RoadmapSpec{
					ResourceName: "/eni/my-api/v1",
					ResourceType: roverv1.ResourceTypeAPI,
					Roadmap:      "test--file-id",
					Hash:         "somehash",
				},
			}

			validator := &RoadmapCustomValidator{}

			warnings, err := validator.ValidateCreate(ctx, roadmap)

			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("metadata.labels.cp.ei.telekom.de/environment"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("environment label is required"))
		})

		It("should block when resourceName is empty", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.RoadmapSpec{
					ResourceName: "", // Empty resourceName
					ResourceType: roverv1.ResourceTypeAPI,
					Roadmap:      "test--file-id",
					Hash:         "somehash",
				},
			}

			validator := &RoadmapCustomValidator{}

			warnings, err := validator.ValidateCreate(ctx, roadmap)

			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.resourceName"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("resourceName must not be empty"))
		})

		It("should block when resourceType is invalid", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.RoadmapSpec{
					ResourceName: "/eni/my-api/v1",
					ResourceType: "InvalidType", // Invalid enum value
					Roadmap:      "test--file-id",
					Hash:         "somehash",
				},
			}

			validator := &RoadmapCustomValidator{}

			warnings, err := validator.ValidateCreate(ctx, roadmap)

			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.resourceType"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("resourceType must be either 'API' or 'Event'"))
		})

		It("should block when roadmap file ID is empty", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.RoadmapSpec{
					ResourceName: "/eni/my-api/v1",
					ResourceType: roverv1.ResourceTypeAPI,
					Roadmap:      "", // Empty file ID
					Hash:         "somehash",
				},
			}

			validator := &RoadmapCustomValidator{}

			warnings, err := validator.ValidateCreate(ctx, roadmap)

			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.roadmap"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("roadmap file ID must not be empty"))
		})

		It("should block when hash is empty", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.RoadmapSpec{
					ResourceName: "/eni/my-api/v1",
					ResourceType: roverv1.ResourceTypeAPI,
					Roadmap:      "test--file-id",
					Hash:         "", // Empty hash
				},
			}

			validator := &RoadmapCustomValidator{}

			warnings, err := validator.ValidateCreate(ctx, roadmap)

			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.hash"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("hash must not be empty"))
		})

		It("should block multiple validation errors at once", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap",
					Namespace: "test--my-group--my-team",
					Labels:    map[string]string{}, // Missing environment label
				},
				Spec: roverv1.RoadmapSpec{
					ResourceName: "",            // Empty resourceName
					ResourceType: "InvalidType", // Invalid resourceType
					Roadmap:      "",            // Empty roadmap
					Hash:         "",            // Empty hash
				},
			}

			validator := &RoadmapCustomValidator{}

			warnings, err := validator.ValidateCreate(ctx, roadmap)

			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			// Should have all 5 validation errors
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(5))
		})
	})
})
