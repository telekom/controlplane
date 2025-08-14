// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"bytes"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
)

var _ = Describe("ApiError", func() {
	var (
		apiErr *common.ApiError
	)

	BeforeEach(func() {
		apiErr = &common.ApiError{
			Type:     "ValidationError",
			Status:   400,
			Title:    "Bad Request",
			Detail:   "Invalid resource configuration",
			Instance: "PUT/resources",
			Fields: []common.FieldError{
				{
					Field:  "spec.replicas",
					Detail: "Field must be greater than 0",
				},
			},
		}
	})

	It("should implement the error interface", func() {
		var _ error = &common.ApiError{}
	})

	Describe("Error", func() {
		It("should return a formatted error message", func() {
			expected := "Bad Request: Invalid resource configuration"
			Expect(apiErr.Error()).To(Equal(expected))
		})
	})

	Describe("IsValidationError", func() {
		It("should return true for ValidationError type", func() {
			apiErr.Type = "ValidationError"
			Expect(common.IsValidationError(apiErr)).To(BeTrue())
		})

		It("should return false for other error types", func() {
			apiErr.Type = "NotFound"
			Expect(common.IsValidationError(apiErr)).To(BeFalse())
		})

		It("should return false for non-ApiError types", func() {
			err := errors.New("generic error")
			Expect(common.IsValidationError(err)).To(BeFalse())
		})
	})

	Describe("AsApiError", func() {
		It("should extract ApiError from error", func() {
			var err error = apiErr
			result, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(result).To(Equal(apiErr))
		})

		It("should return false for non-ApiError types", func() {
			err := errors.New("generic error")
			result, ok := common.AsApiError(err)
			Expect(ok).To(BeFalse())
			Expect(result).To(BeNil())
		})

		It("should extract ApiError from wrapped error", func() {
			var err error = apiErr
			// Create wrapped error without using errors.Wrap
			wrappedErr := fmt.Errorf("wrapped error: %w", err)
			result, ok := common.AsApiError(wrappedErr)
			Expect(ok).To(BeTrue())
			Expect(result).To(Equal(apiErr))
		})
	})

	Describe("PrintTo", func() {
		It("should call PrintJsonTo for json format", func() {
			var buf bytes.Buffer
			common.PrintTo(apiErr, &buf, "json")
			output := buf.String()

			// Should contain JSON elements
			Expect(output).To(ContainSubstring("\"type\":"))
			Expect(output).To(ContainSubstring("\"status\":"))
			Expect(output).NotTo(ContainSubstring("❌ Error"))
		})

		It("should call PrintTextTo for text format", func() {
			var buf bytes.Buffer
			common.PrintTo(apiErr, &buf, "text")
			output := buf.String()

			// Should contain text elements
			Expect(output).To(ContainSubstring("❌ Error"))
			Expect(output).To(ContainSubstring("Type: ValidationError"))
		})

		It("should default to text format for unknown formats", func() {
			var buf bytes.Buffer
			common.PrintTo(apiErr, &buf, "unknown")
			output := buf.String()

			// Should contain text elements
			Expect(output).To(ContainSubstring("❌ Error"))
			Expect(output).To(ContainSubstring("Type: ValidationError"))
		})
	})

	Describe("PrintTextTo", func() {
		It("should format ApiError as text", func() {
			var buf bytes.Buffer
			common.PrintTextTo(apiErr, &buf)
			output := buf.String()

			Expect(output).To(ContainSubstring("❌ Error"))
			Expect(output).To(ContainSubstring("Type: ValidationError"))
			Expect(output).To(ContainSubstring("Status: 400"))
			Expect(output).To(ContainSubstring("Title: Bad Request"))
			Expect(output).To(ContainSubstring("Detail: Invalid resource configuration"))
			Expect(output).To(ContainSubstring("Instance: PUT/resources"))
			Expect(output).To(ContainSubstring("Fields:"))
			Expect(output).To(ContainSubstring("Field: spec.replicas"))
			Expect(output).To(ContainSubstring("Detail: Field must be greater than 0"))
		})

		It("should format regular error as ApiError text", func() {
			var buf bytes.Buffer
			err := errors.New("generic error")
			common.PrintTextTo(err, &buf)
			output := buf.String()

			Expect(output).To(ContainSubstring("❌ Error"))
			Expect(output).To(ContainSubstring("Type: InternalError"))
			Expect(output).To(ContainSubstring("Status: 500"))
			Expect(output).To(ContainSubstring("Title: Internal Server Error"))
			Expect(output).To(ContainSubstring("Detail: generic error"))
			Expect(output).NotTo(ContainSubstring("Fields:"))
		})

		It("should not include Instance field if empty", func() {
			var buf bytes.Buffer
			apiErr.Instance = ""
			common.PrintTextTo(apiErr, &buf)
			output := buf.String()

			Expect(output).NotTo(ContainSubstring("Instance:"))
		})

		It("should not include Fields section if empty", func() {
			var buf bytes.Buffer
			apiErr.Fields = nil
			common.PrintTextTo(apiErr, &buf)
			output := buf.String()

			Expect(output).NotTo(ContainSubstring("Fields:"))
		})
	})

	Describe("PrintJsonTo", func() {
		It("should format ApiError as JSON", func() {
			var buf bytes.Buffer
			common.PrintJsonTo(apiErr, &buf)
			output := buf.String()

			Expect(output).To(ContainSubstring("\"type\": \"ValidationError\""))
			Expect(output).To(ContainSubstring("\"status\": 400"))
			Expect(output).To(ContainSubstring("\"title\": \"Bad Request\""))
			Expect(output).To(ContainSubstring("\"detail\": \"Invalid resource configuration\""))
			Expect(output).To(ContainSubstring("\"instance\": \"PUT/resources\""))
			Expect(output).To(ContainSubstring("\"fields\": ["))
			Expect(output).To(ContainSubstring("\"field\": \"spec.replicas\""))
			Expect(output).To(ContainSubstring("\"detail\": \"Field must be greater than 0\""))
		})

		It("should format regular error as ApiError JSON", func() {
			var buf bytes.Buffer
			err := errors.New("generic error")
			common.PrintJsonTo(err, &buf)
			output := buf.String()

			Expect(output).To(ContainSubstring("\"type\": \"InternalError\""))
			Expect(output).To(ContainSubstring("\"status\": 500"))
			Expect(output).To(ContainSubstring("\"title\": \"Internal Server Error\""))
			Expect(output).To(ContainSubstring("\"detail\": \"generic error\""))
			Expect(output).NotTo(ContainSubstring("\"fields\":"))
		})

		It("should omit fields array when empty", func() {
			var buf bytes.Buffer
			apiErr.Fields = nil
			common.PrintJsonTo(apiErr, &buf)
			output := buf.String()

			Expect(output).NotTo(ContainSubstring("\"fields\":"))
		})
	})
})
