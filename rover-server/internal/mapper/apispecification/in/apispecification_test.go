// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"context"

	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"gopkg.in/yaml.v3"
)

var _ = Describe("ApiSpecification Mapper", func() {
	Context("MapRequest", func() {
		It("must map a ApiSpecificationUpdateRequest to an ApiSpecification correctly", func() {
			// Create a context with business context
			ctx := context.WithValue(context.Background(), "businessContext", &security.BusinessContext{
				Environment: "poc",
				Group:       "eni",
				Team:        "hyperion",
			})

			// Create a mock FileUploadResponse
			fileAPIResp := &filesapi.FileUploadResponse{
				FileHash:    "test-hash",
				FileId:      "test-file-id",
				ContentType: "application/yaml",
			}

			marshalled, err := yaml.Marshal(apiSpecification.Specification)
			Expect(err).To(BeNil())

			output, err := MapRequest(ctx, marshalled, fileAPIResp, resourceIdInfo)
			Expect(err).To(BeNil())

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error if the input ApiSpecificationUpdateRequest is nil", func() {
			// Create a context with business context
			ctx := context.WithValue(context.Background(), "businessContext", &security.BusinessContext{
				Environment: "poc",
				Group:       "eni",
				Team:        "hyperion",
			})

			// Create a mock FileUploadResponse
			fileAPIResp := &filesapi.FileUploadResponse{
				FileHash:    "test-hash",
				FileId:      "test-file-id",
				ContentType: "application/yaml",
			}

			output, err := MapRequest(ctx, nil, fileAPIResp, resourceIdInfo)

			Expect(output).To(BeNil())

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input api specification is nil"))
		})
	})
})
