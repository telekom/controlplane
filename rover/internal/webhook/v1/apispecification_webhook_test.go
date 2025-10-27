// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ApiSpecification Webhook", func() {

	Context("Validating", func() {

		var ctx = context.Background()
		var environment = "test"

		var NewApiSpecificationValidatorMock = func(teamGroup, teamName string) *ApiSpecificationCustomValidator {
			apiCategoriesList := &apiv1.ApiCategoryList{
				Items: []apiv1.ApiCategory{
					{
						Spec: apiv1.ApiCategorySpec{
							Active:              true,
							LabelValue:          "some-api-category",
							MustHaveGroupPrefix: true,
							AllowTeams: &apiv1.AllowTeamsConfig{
								Categories: []string{"*"},
								Names:      []string{"*"},
							},
						},
					},
					{
						Spec: apiv1.ApiCategorySpec{
							Active:              true,
							LabelValue:          "other-api-category",
							MustHaveGroupPrefix: false,
						},
					},
					{
						Spec: apiv1.ApiCategorySpec{
							Active:              false,
							LabelValue:          "inactive-api-category",
							MustHaveGroupPrefix: false,
						},
					},
					{
						Spec: apiv1.ApiCategorySpec{
							Active:              true,
							LabelValue:          "not-allowed-api-category",
							MustHaveGroupPrefix: false,
							AllowTeams: &apiv1.AllowTeamsConfig{
								Categories: []string{"not-allowed-team-category"},
								Names:      []string{"not-allowed-team"},
							},
						},
					},
				},
			}

			team := &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      teamGroup + "--" + teamName,
					Namespace: environment,
				},
				Spec: organizationv1.TeamSpec{
					Group:    teamGroup,
					Category: organizationv1.TeamCategoryCustomer,
				},
			}

			return &ApiSpecificationCustomValidator{
				FindTeam: func(ctx context.Context, obj types.NamedObject) (*organizationv1.Team, error) {
					return team, nil
				},
				ListApiCategories: func(ctx context.Context) (*apiv1.ApiCategoryList, error) {
					return apiCategoriesList, nil
				},
			}

		}

		It("should block when the environment label is missing", func() {
			By("creating an ApiSpecification without the environment label")
			apispecification := &roverv1.ApiSpecification{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-api-specification",
					Namespace: "test--my-group--my-team", // team with group "my-group"
					Labels:    map[string]string{},
				},
				Spec: roverv1.ApiSpecificationSpec{
					Category: "some-api-category",
				},
			}

			validator := NewApiSpecificationValidatorMock("my-group", "my-team")

			By("validating the ApiSpecification")
			warnings, err := validator.ValidateCreate(ctx, apispecification)

			By("expecting an error about the missing environment label")
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("metadata.labels.cp.ei.telekom.de/environment"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("environment label is required"))
		})

		It("should block when the ApiCategory does not exist", func() {
			By("creating an ApiSpecification with a non-existing ApiCategory")
			apispecification := &roverv1.ApiSpecification{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-api-specification",
					Namespace: "test--my-group--my-team", // team with group "my-group"
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ApiSpecificationSpec{
					Version:  "1.0.0",
					BasePath: "/my-group/my-api/v1",
					Category: "not-existing-category", // does not exist
				},
			}

			validator := NewApiSpecificationValidatorMock("my-group", "my-team")

			By("validating the ApiSpecification")
			warnings, err := validator.ValidateCreate(ctx, apispecification)

			By("expecting an error about the non-existing ApiCategory")
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.category"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring(`ApiCategory "not-existing-category" not found. Allowed values are: [not-allowed-api-category, other-api-category, some-api-category]`))
		})

		It("should return an error when the group prefix is required but not set", func() {
			By("creating an ApiSpecification with a ApiCategory that requires the group prefix but not setting it")
			apispecification := &roverv1.ApiSpecification{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-api-specification",
					Namespace: "test--my-group--my-team", // team with group "my-group"
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ApiSpecificationSpec{
					Version:  "1.0.0",
					Category: "some-api-category",      // requires group prefix
					BasePath: "/wrong-group/my-api/v1", // does not start with "my-group"
				},
			}

			validator := NewApiSpecificationValidatorMock("my-group", "my-team")

			By("validating the ApiSpecification")
			warnings, err := validator.ValidateCreate(ctx, apispecification)

			By("expecting an error about the missing group prefix in the basePath")
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.basePath"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring(`basePath must start with the team group prefix "my-group" as ApiCategory "some-api-category" requires it`))
		})

		It("should return an error when the ApiCategory is inactive", func() {
			By("creating an ApiSpecification with a ApiCategory that is inactive")
			apispecification := &roverv1.ApiSpecification{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-api-specification",
					Namespace: "test--my-group--my-team", // team with group "my-group"
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ApiSpecificationSpec{
					Version:  "1.0.0",
					Category: "inactive-api-category", // is inactive
					BasePath: "/my-group/my-api/v1",   // starts with "my-group"
				},
			}

			validator := NewApiSpecificationValidatorMock("my-group", "my-team")

			By("validating the ApiSpecification")
			warnings, err := validator.ValidateCreate(ctx, apispecification)

			By("expecting an error about the inactive ApiCategory")
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(1))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.category"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("the provided ApiCategory is not active"))
		})

		It("should succeed when the ApiCategory exists and the group prefix is correctly set", func() {
			By("creating an ApiSpecification with a valid ApiCategory and correct group prefix")
			apispecification := &roverv1.ApiSpecification{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-api-specification",
					Namespace: "test--my-group--my-team", // team with group "my-group"
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ApiSpecificationSpec{
					Version:  "1.0.0",
					Category: "some-api-category",   // exists and requires group prefix
					BasePath: "/my-group/my-api/v1", // starts with "my-group"
				},
			}

			validator := NewApiSpecificationValidatorMock("my-group", "my-team")

			By("validating the ApiSpecification")
			warnings, err := validator.ValidateCreate(ctx, apispecification)

			By("expecting no error")
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("should allow any ApiCategory when no ApiCategories are defined in the environment", func() {
			By("creating an ApiSpecification with any ApiCategory when no ApiCategories exist")
			apispecification := &roverv1.ApiSpecification{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-api-specification",
					Namespace: "test--my-group--my-team", // team with group "my-group"
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ApiSpecificationSpec{
					Version:  "1.0.0",
					Category: "any-category", // does not exist but should be allowed
					BasePath: "/any-group/my-api/v1",
				},
			}

			validator := NewApiSpecificationValidatorMock("my-group", "my-team")
			validator.ListApiCategories = func(_ context.Context) (*apiv1.ApiCategoryList, error) {
				return nil, nil // noop
			}

			By("validating the ApiSpecification")
			warnings, err := validator.ValidateCreate(ctx, apispecification)

			By("expecting no error")
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())

		})

		It("should block when the Team is not allowed", func() {
			By("creating an ApiSpecification with a ApiCategory that is not allowed for the Team")
			apispecification := &roverv1.ApiSpecification{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-api-specification",
					Namespace: "test--my-group--my-team", // team with group "my-group"
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.ApiSpecificationSpec{
					Version:  "1.0.0",
					Category: "not-allowed-api-category", // exists but not allowed for the team
					BasePath: "/any-group/my-api/v1",
				},
			}

			validator := NewApiSpecificationValidatorMock("my-group", "my-team")

			By("validating the ApiSpecification")
			warnings, err := validator.ValidateCreate(ctx, apispecification)

			By("expecting an error about the not allowed ApiCategory")
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "Expected an Invalid error")

			statusErr, ok := err.(*apierrors.StatusError)
			Expect(ok).To(BeTrue(), "Expected a StatusError, got: %T", err)
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(2))
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.category"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring(`ApiCategory "not-allowed-api-category" is not allowed for team category "Customer"`))

			Expect(statusErr.ErrStatus.Details.Causes[1].Field).To(Equal("spec.category"))
			Expect(statusErr.ErrStatus.Details.Causes[1].Message).To(ContainSubstring(`ApiCategory "not-allowed-api-category" is not allowed for team name "my-group--my-team"`))

		})
	})

})
