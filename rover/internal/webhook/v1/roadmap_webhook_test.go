// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Roadmap Webhook", func() {

	Context("Validating", func() {

		var ctx = context.Background()

		It("should allow valid Roadmap with specification reference", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.RoadmapSpec{
					SpecificationRef: types.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ApiSpecification",
							APIVersion: "rover.cp.ei.telekom.de/v1",
						},
						ObjectRef: types.ObjectRef{
							Name:      "eni-my-api",
							Namespace: "test--my-group--my-team",
						},
					},
					Contents: "test--eni--team--my-api-v1",
					Hash:     "abc123hash",
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
					SpecificationRef: types.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ApiSpecification",
							APIVersion: "rover.cp.ei.telekom.de/v1",
						},
						ObjectRef: types.ObjectRef{
							Name:      "eni-my-api",
							Namespace: "test--my-group--my-team",
						},
					},
					Contents: "test--file-id",
					Hash:     "somehash",
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

		It("should block when specification name is empty", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.RoadmapSpec{
					SpecificationRef: types.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ApiSpecification",
							APIVersion: "rover.cp.ei.telekom.de/v1",
						},
						ObjectRef: types.ObjectRef{
							Name:      "", // Empty name
							Namespace: "test--my-group--my-team",
						},
					},
					Contents: "test--file-id",
					Hash:     "somehash",
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
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.specificationRef.name"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("specificationRef name must not be empty"))
		})

		It("should block when specification namespace is empty", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.RoadmapSpec{
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
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.specificationRef.namespace"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("specificationRef namespace must not be empty"))
		})

		It("should block when contents file ID is empty", func() {
			roadmap := &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roadmap",
					Namespace: "test--my-group--my-team",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.RoadmapSpec{
					SpecificationRef: types.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ApiSpecification",
							APIVersion: "rover.cp.ei.telekom.de/v1",
						},
						ObjectRef: types.ObjectRef{
							Name:      "eni-my-api",
							Namespace: "test--my-group--my-team",
						},
					},
					Contents: "", // Empty file ID
					Hash:     "somehash",
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
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.contents"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("contents file ID must not be empty"))
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
					SpecificationRef: types.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ApiSpecification",
							APIVersion: "rover.cp.ei.telekom.de/v1",
						},
						ObjectRef: types.ObjectRef{
							Name:      "eni-my-api",
							Namespace: "test--my-group--my-team",
						},
					},
					Contents: "test--file-id",
					Hash:     "", // Empty hash
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
					SpecificationRef: types.TypedObjectRef{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ApiSpecification",
							APIVersion: "rover.cp.ei.telekom.de/v1",
						},
						ObjectRef: types.ObjectRef{
							Name:      "", // Empty name
							Namespace: "", // Empty namespace
						},
					},
					Contents: "", // Empty contents
					Hash:     "", // Empty hash
				},
			}

			validator := &RoadmapCustomValidator{}

			warnings, err := validator.ValidateCreate(ctx, roadmap)

			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			// Should have all 5 validation errors: env label, apispec name, apispec namespace, contents, hash
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(5))
		})
	})
})
