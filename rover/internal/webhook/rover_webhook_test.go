// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func NewRover(zone adminv1.Zone) *roverv1.Rover {
	return &roverv1.Rover{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Rover",
			APIVersion: roverv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rover",
			Namespace: "default",
			Labels: map[string]string{
				config.EnvironmentLabelKey: zone.Namespace,
			},
		},
		Spec: roverv1.RoverSpec{
			Zone: zone.Name,
			Exposures: []roverv1.Exposure{
				{
					Api: &roverv1.ApiExposure{
						Approval: roverv1.Approval{
							Strategy: roverv1.ApprovalStrategyAuto,
						},
						BasePath:   "/eni/distr/v1",
						Visibility: "World",
						Upstreams: []roverv1.Upstream{
							{URL: "https://upstream1.example.com"},
						},
					},
				},
			},
		},
	}
}

var _ = Describe("Rover Webhook", Ordered, func() {
	var (
		roverObj  *roverv1.Rover
		validator RoverValidator
		defaulter RoverDefaulter
	)

	BeforeEach(func() {
		roverObj = &roverv1.Rover{}
		validator = RoverValidator{k8sClient}
		defaulter = RoverDefaulter{k8sClient}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(roverObj).NotTo(BeNil(), "Expected roverObj to be initialized")

	})

	Context("When creating Rover under Defaulting Webhook", func() {
		It("Should fill in the default value if a required field is empty", func() {
			// TODO Update test once the defaulter is implemented
			err := defaulter.Default(ctx, roverObj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When creating Rover under Validating Webhook", func() {
		It("Should deny if a required field is empty", func() {
			By("Creating a Rover without an environment")
			roverObj = &roverv1.Rover{
				Spec: roverv1.RoverSpec{},
			}
			warnings, err := validator.ValidateCreate(ctx, roverObj)
			expectedErrorMessage := "environment not found"
			assertValidationFailedWith(warnings, err, errors.IsBadRequest, expectedErrorMessage)

			By("Creating a Rover multiple upstreams and only one contains weight")
			roverObj = NewRover(*testZone)
			roverObj.Spec.Exposures[0].Api.Upstreams = []roverv1.Upstream{
				{URL: "https://upstream1.example.com", Weight: 1},
				{URL: "https://upstream2.example.com"},
			}
			warnings, err = validator.ValidateCreate(ctx, roverObj)
			expectedErrorMessage = "all upstreams must have a weight set or none must have a weight set"
			assertValidationFailedWith(warnings, err, errors.IsBadRequest, expectedErrorMessage)

		})

		It("Should admit if all required fields are provided", func() {
			By("Creating a Rover with 1 upstream")
			roverObj = NewRover(*testZone)
			warnings, err := validator.ValidateCreate(ctx, roverObj)
			Expect(warnings).To(BeNil())
			Expect(err).ToNot(HaveOccurred())

			By("Creating a Rover with 2 upstreams and no weights")
			roverObj = NewRover(*testZone)
			roverObj.Spec.Exposures[0].Api.Upstreams = []roverv1.Upstream{
				{URL: "https://upstream1.example.com"},
				{URL: "https://upstream2.example.com"},
			}
			warnings, err = validator.ValidateCreate(ctx, roverObj)
			Expect(warnings).To(BeNil())
			Expect(err).ToNot(HaveOccurred())

			By("Creating a Rover with 2 upstreams and 2 weights")
			roverObj = NewRover(*testZone)
			roverObj.Spec.Exposures[0].Api.Upstreams = []roverv1.Upstream{
				{URL: "https://upstream1.example.com", Weight: 1},
				{URL: "https://upstream2.example.com", Weight: 2},
			}
			warnings, err = validator.ValidateCreate(ctx, roverObj)
			Expect(warnings).To(BeNil())
			Expect(err).ToNot(HaveOccurred())

			By("Creating a Rover with remove headers = Authorization and zone visibility is not World")
			roverObj = NewRover(*testZone)
			roverObj.Spec.Exposures[0].Api.Transformation = &roverv1.Transformation{
				Request: roverv1.RequestResponseTransformation{
					Headers: roverv1.HeaderTransformation{
						Remove: []string{"Authorization"},
					},
				},
			}

			warnings, err = validator.ValidateCreate(ctx, roverObj)
			Expect(warnings).To(BeNil())
			Expect(err).ToNot(HaveOccurred())

		})
	})

	Context("When validating trusted teams in approval", func() {
		var (
			team1, team2 *organizationv1.Team
		)

		BeforeAll(func() {
			// Create test teams
			team1 = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "trusted-group-1--trusted-team-1",
					Namespace: testNamespace,
				},
				Spec: organizationv1.TeamSpec{
					Name:  "trusted-team-1",
					Group: "trusted-group-1",
					Email: "team1@example.com",
					Members: []organizationv1.Member{
						{
							Name:  "name",
							Email: "name@example.com",
						},
					},
				},
			}

			team2 = &organizationv1.Team{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "trusted-group-2--trusted-team-2",
					Namespace: testNamespace,
				},
				Spec: organizationv1.TeamSpec{
					Name:  "trusted-team-2",
					Group: "trusted-group-2",
					Email: "team2@example.com",
					Members: []organizationv1.Member{
						{
							Name:  "name",
							Email: "name@example.com",
						},
					},
				},
			}

			// Create the teams in the test environment
			Expect(k8sClient.Create(ctx, team1)).To(Succeed())
			Expect(k8sClient.Create(ctx, team2)).To(Succeed())
		})

		AfterAll(func() {
			// Clean up the teams
			Expect(k8sClient.Delete(ctx, team1)).To(Succeed())
			Expect(k8sClient.Delete(ctx, team2)).To(Succeed())
		})

		It("Should validate when all trusted teams exist", func() {
			approval := roverv1.Approval{
				Strategy: roverv1.ApprovalStrategyFourEyes,
				TrustedTeams: []roverv1.TrustedTeam{
					{
						Group: "trusted-group-1",
						Team:  "trusted-team-1",
					},
					{
						Group: "trusted-group-2",
						Team:  "trusted-team-2",
					},
				},
			}

			err := validator.validateApproval(ctx, testNamespace, approval)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should validate when trusted teams list is empty", func() {
			approval := roverv1.Approval{
				Strategy:     roverv1.ApprovalStrategySimple,
				TrustedTeams: []roverv1.TrustedTeam{},
			}

			err := validator.validateApproval(ctx, testNamespace, approval)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should fail validation when a trusted team doesn't exist", func() {
			approval := roverv1.Approval{
				Strategy: roverv1.ApprovalStrategyFourEyes,
				TrustedTeams: []roverv1.TrustedTeam{
					{
						Group: "nonexistent-group",
						Team:  "nonexistent-team",
					},
				},
			}

			err := validator.validateApproval(ctx, testNamespace, approval)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Team not found"))
		})

		It("Should validate the trusted teams in a Rover resource", func() {
			roverObj := NewRover(*testZone)
			roverObj.Spec.Exposures[0].Api.Approval.TrustedTeams = []roverv1.TrustedTeam{
				{
					Group: "trusted-group-1",
					Team:  "trusted-team-1",
				},
			}

			warnings, err := validator.ValidateCreate(ctx, roverObj)
			Expect(warnings).To(BeNil())
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should fail validation when a Rover contains non-existent trusted teams", func() {
			roverObj := NewRover(*testZone)
			roverObj.Spec.Exposures[0].Api.Approval.TrustedTeams = []roverv1.TrustedTeam{
				{
					Group: "nonexistent-group",
					Team:  "nonexistent-team",
				},
			}

			warnings, err := validator.ValidateCreate(ctx, roverObj)
			Expect(warnings).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Team not found"))
		})
	})

})

func assertValidationFailedWith(warnings admission.Warnings, err error, isErrorType func(error) bool, expectedErrorMessage string) {
	Expect(warnings).To(BeNil())
	Expect(err).To(HaveOccurred())
	Expect(isErrorType(err)).To(BeTrue())
	Expect(err.Error()).To(ContainSubstring(expectedErrorMessage))
}
