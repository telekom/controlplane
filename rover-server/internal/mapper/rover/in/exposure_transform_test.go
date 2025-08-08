// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Exposure Transformation Mapper", func() {
	Context("mapExposureTransformation", func() {
		It("must map removeHeaders correctly", func() {
			// Given
			input := api.ApiExposure{
				BasePath:      "/test",
				RemoveHeaders: []string{"X-Test-Header", "Authorization"},
			}

			output := &roverv1.ApiExposure{}

			// When
			mapExposureTransformation(input, output)

			// Then
			Expect(output.Transformation).ToNot(BeNil())
			Expect(output.Transformation.Request.Headers.Remove).To(HaveLen(2))
			Expect(output.Transformation.Request.Headers.Remove).To(ContainElements("X-Test-Header", "Authorization"))
			snaps.MatchSnapshot(GinkgoT(), output.Transformation)
		})

		It("must handle nil removeHeaders", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
				// RemoveHeaders is nil
			}

			output := &roverv1.ApiExposure{}

			// When
			mapExposureTransformation(input, output)

			// Then
			Expect(output.Transformation).To(BeNil())
		})

		It("must handle empty removeHeaders", func() {
			// Given
			input := api.ApiExposure{
				BasePath:      "/test",
				RemoveHeaders: []string{},
			}

			output := &roverv1.ApiExposure{}

			// When
			mapExposureTransformation(input, output)

			// Then
			// Empty removeHeaders still creates a Transformation object
			Expect(output.Transformation).ToNot(BeNil())
			Expect(output.Transformation.Request.Headers.Remove).To(BeEmpty())
		})
	})
})
