// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
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

			By("Creating a Rover with more than 12 upstreams")
			roverObj = NewRover(*testZone)
			roverObj.Spec.Exposures[0].Api.Upstreams = []roverv1.Upstream{
				{URL: "https://upstream1.example.com"},
				{URL: "https://upstream2.example.com"},
				{URL: "https://upstream3.example.com"},
				{URL: "https://upstream4.example.com"},
				{URL: "https://upstream5.example.com"},
				{URL: "https://upstream6.example.com"},
				{URL: "https://upstream7.example.com"},
				{URL: "https://upstream8.example.com"},
				{URL: "https://upstream9.example.com"},
				{URL: "https://upstream10.example.com"},
				{URL: "https://upstream11.example.com"},
				{URL: "https://upstream12.example.com"},
				{URL: "https://upstream13.example.com"},
			}
			warnings, err = validator.ValidateCreate(ctx, roverObj)
			expectedErrorMessage = "maximum of 12 upstreams allowed"
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

		})
	})

})

func assertValidationFailedWith(warnings admission.Warnings, err error, isErrorType func(error) bool, expectedErrorMessage string) {
	Expect(warnings).To(BeNil())
	Expect(err).To(HaveOccurred())
	Expect(isErrorType(err)).To(BeTrue())
	Expect(err.Error()).To(ContainSubstring(expectedErrorMessage))
}
