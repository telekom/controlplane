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

var _ = Describe("Changelog Webhook", func() {
	Context("Validating", func() {
		var ctx = context.Background()

		It("should block when the environment label is missing", func() {
			By("creating a Changelog without the environment label")
			changelog := &roverv1.Changelog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-changelog",
					Namespace: "test--my-group--my-team",
					Labels:    map[string]string{},
				},
				Spec: roverv1.ChangelogSpec{
					ResourceName: "/eni/test-api/v1",
					ResourceType: roverv1.ResourceTypeAPI,
					Changelog:    "file-id",
					Hash:         "hash123",
				},
			}

			validator := &ChangelogCustomValidator{}

			By("validating the Changelog")
			warnings, err := validator.ValidateCreate(ctx, changelog)

			By("expecting an error about the missing environment label")
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue())
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("metadata.labels.cp.ei.telekom.de/environment"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("environment label is required"))
		})

		It("should block when resourceName is missing", func() {
			By("creating a Changelog without resourceName")
			changelog := &roverv1.Changelog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-changelog",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ChangelogSpec{
					ResourceName: "",
					ResourceType: roverv1.ResourceTypeAPI,
					Changelog:    "file-id",
					Hash:         "hash123",
				},
			}

			validator := &ChangelogCustomValidator{}
			warnings, err := validator.ValidateCreate(ctx, changelog)

			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue())
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.resourceName"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("resourceName is required"))
		})

		It("should block when resourceType is invalid", func() {
			By("creating a Changelog with invalid resourceType")
			changelog := &roverv1.Changelog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-changelog",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ChangelogSpec{
					ResourceName: "/eni/test-api/v1",
					ResourceType: "INVALID",
					Changelog:    "file-id",
					Hash:         "hash123",
				},
			}

			validator := &ChangelogCustomValidator{}
			warnings, err := validator.ValidateCreate(ctx, changelog)

			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue())
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.resourceType"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("resourceType must be either 'API' or 'Event'"))
		})

		It("should block when changelog file reference is missing", func() {
			By("creating a Changelog without changelog fileId")
			changelog := &roverv1.Changelog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-changelog",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ChangelogSpec{
					ResourceName: "/eni/test-api/v1",
					ResourceType: roverv1.ResourceTypeAPI,
					Changelog:    "",
					Hash:         "hash123",
				},
			}

			validator := &ChangelogCustomValidator{}
			warnings, err := validator.ValidateCreate(ctx, changelog)

			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue())
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.changelog"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("changelog file reference is required"))
		})

		It("should block when hash is missing", func() {
			By("creating a Changelog without hash")
			changelog := &roverv1.Changelog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-changelog",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ChangelogSpec{
					ResourceName: "/eni/test-api/v1",
					ResourceType: roverv1.ResourceTypeAPI,
					Changelog:    "file-id",
					Hash:         "",
				},
			}

			validator := &ChangelogCustomValidator{}
			warnings, err := validator.ValidateCreate(ctx, changelog)

			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue())
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.hash"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("hash is required"))
		})

		It("should succeed when all required fields are provided", func() {
			By("creating a valid Changelog")
			changelog := &roverv1.Changelog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-changelog",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ChangelogSpec{
					ResourceName: "/eni/test-api/v1",
					ResourceType: roverv1.ResourceTypeAPI,
					Changelog:    "file-id",
					Hash:         "hash123",
				},
			}

			validator := &ChangelogCustomValidator{}
			warnings, err := validator.ValidateCreate(ctx, changelog)

			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("should succeed for Event resourceType", func() {
			By("creating a valid Changelog for an Event")
			changelog := &roverv1.Changelog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-event-changelog",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ChangelogSpec{
					ResourceName: "com.telekom.example.event.v1",
					ResourceType: roverv1.ResourceTypeEvent,
					Changelog:    "file-id",
					Hash:         "hash123",
				},
			}

			validator := &ChangelogCustomValidator{}
			warnings, err := validator.ValidateCreate(ctx, changelog)

			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})
	})
})
