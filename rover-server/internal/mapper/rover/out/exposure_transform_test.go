// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Exposure Transformation Mapper (Out)", func() {
	Context("mapExposureTransformation", func() {
		It("must map request headers transformation correctly", func() {
			// Given
			input := &roverv1.ApiExposure{
				BasePath: "/test",
				Transformation: &roverv1.Transformation{
					Request: roverv1.RequestResponseTransformation{
						Headers: roverv1.HeaderTransformation{
							Remove: []string{"X-Test-Header", "Authorization"},
						},
					},
				},
			}

			output := &api.ApiExposure{}

			// When
			mapExposureTransformation(input, output)

			// Then
			Expect(output.RemoveHeaders).ToNot(BeNil())
			Expect(output.RemoveHeaders).To(HaveLen(2))
			Expect(output.RemoveHeaders).To(ContainElements("X-Test-Header", "Authorization"))
			snaps.MatchSnapshot(GinkgoT(), output.RemoveHeaders)
		})

		It("must handle nil transformation", func() {
			// Given
			input := &roverv1.ApiExposure{
				BasePath: "/test",
				// Transformation is nil
			}

			output := &api.ApiExposure{}

			// When
			mapExposureTransformation(input, output)

			// Then
			Expect(output.RemoveHeaders).To(BeNil())
		})

		It("must handle nil headers remove", func() {
			// Given
			input := &roverv1.ApiExposure{
				BasePath: "/test",
				Transformation: &roverv1.Transformation{
					Request: roverv1.RequestResponseTransformation{
						Headers: roverv1.HeaderTransformation{
							// Remove is nil
						},
					},
				},
			}

			output := &api.ApiExposure{}

			// When
			mapExposureTransformation(input, output)

			// Then
			Expect(output.RemoveHeaders).To(BeNil())
		})

		It("must handle empty headers remove", func() {
			// Given
			input := &roverv1.ApiExposure{
				BasePath: "/test",
				Transformation: &roverv1.Transformation{
					Request: roverv1.RequestResponseTransformation{
						Headers: roverv1.HeaderTransformation{
							Remove: []string{},
						},
					},
				},
			}

			output := &api.ApiExposure{}

			// When
			mapExposureTransformation(input, output)

			// Then
			Expect(output.RemoveHeaders).To(BeNil())
		})
	})
})
