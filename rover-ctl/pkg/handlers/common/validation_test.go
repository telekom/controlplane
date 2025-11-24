// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ = Describe("ValidateObjectName", func() {
	var createTestObject = func(name string, filename ...string) types.Object {
		obj := &types.UnstructuredObject{
			Content: map[string]any{
				"apiVersion": "v1",
				"kind":       "Test",
				"metadata": map[string]any{
					"name": name,
				},
			},
		}
		if len(filename) > 0 && filename[0] != "" {
			obj.SetProperty("filename", filename[0])
		}
		return obj
	}

	Context("with valid names", func() {
		It("should accept simple lowercase names", func() {
			obj := createTestObject("simple")
			err := common.ValidateObjectName(obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names with numbers", func() {
			obj := createTestObject("name123")
			err := common.ValidateObjectName(obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names with single hyphens", func() {
			obj := createTestObject("valid-name-with-hyphens")
			err := common.ValidateObjectName(obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names starting with numbers", func() {
			obj := createTestObject("123name")
			err := common.ValidateObjectName(obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names ending with numbers", func() {
			obj := createTestObject("name123")
			err := common.ValidateObjectName(obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept single character names", func() {
			obj := createTestObject("a")
			err := common.ValidateObjectName(obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names with exactly 63 characters", func() {
			validName := strings.Repeat("a", 63)
			obj := createTestObject(validName)
			err := common.ValidateObjectName(obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept names with alternating letters and numbers", func() {
			obj := createTestObject("a1b2c3d4")
			err := common.ValidateObjectName(obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("with invalid names", func() {
		It("should reject names longer than 63 characters", func() {
			longName := strings.Repeat("a", 64)
			obj := createTestObject(longName)
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Type).To(Equal("ValidationError"))
			Expect(apiErr.Status).To(Equal(400))
			Expect(apiErr.Title).To(ContainSubstring("Failed to validate object"))
			Expect(apiErr.Fields).To(HaveLen(1))
			Expect(apiErr.Fields[0].Field).To(Equal("name"))
			Expect(apiErr.Fields[0].Detail).To(Equal("name must be no more than 63 characters"))
		})

		It("should reject names with uppercase letters", func() {
			obj := createTestObject("InvalidName")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(1))
			Expect(apiErr.Fields[0].Field).To(Equal("name"))
			Expect(apiErr.Fields[0].Detail).To(Equal("name must consist of lower case alphanumeric characters or '-', start and end with an alphanumeric character"))
		})

		It("should reject names starting with hyphens", func() {
			obj := createTestObject("-invalid")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(1))
			Expect(apiErr.Fields[0].Field).To(Equal("name"))
			Expect(apiErr.Fields[0].Detail).To(Equal("name must consist of lower case alphanumeric characters or '-', start and end with an alphanumeric character"))
		})

		It("should reject names ending with hyphens", func() {
			obj := createTestObject("invalid-")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(1))
			Expect(apiErr.Fields[0].Field).To(Equal("name"))
			Expect(apiErr.Fields[0].Detail).To(Equal("name must consist of lower case alphanumeric characters or '-', start and end with an alphanumeric character"))
		})

		It("should reject names with consecutive hyphens", func() {
			obj := createTestObject("invalid--name")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(1))
			Expect(apiErr.Fields[0].Field).To(Equal("name"))
			Expect(apiErr.Fields[0].Detail).To(Equal("name must not contain consecutive '-' characters"))
		})

		It("should reject names with special characters", func() {
			obj := createTestObject("invalid@name")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(1))
			Expect(apiErr.Fields[0].Field).To(Equal("name"))
			Expect(apiErr.Fields[0].Detail).To(Equal("name must consist of lower case alphanumeric characters or '-', start and end with an alphanumeric character"))
		})

		It("should reject names with underscores", func() {
			obj := createTestObject("invalid_name")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(1))
			Expect(apiErr.Fields[0].Field).To(Equal("name"))
			Expect(apiErr.Fields[0].Detail).To(Equal("name must consist of lower case alphanumeric characters or '-', start and end with an alphanumeric character"))
		})

		It("should reject names with dots", func() {
			obj := createTestObject("invalid.name")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(1))
			Expect(apiErr.Fields[0].Field).To(Equal("name"))
			Expect(apiErr.Fields[0].Detail).To(Equal("name must consist of lower case alphanumeric characters or '-', start and end with an alphanumeric character"))
		})

		It("should reject empty names", func() {
			obj := createTestObject("")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(1))
			Expect(apiErr.Fields[0].Field).To(Equal("name"))
			Expect(apiErr.Fields[0].Detail).To(Equal("name must consist of lower case alphanumeric characters or '-', start and end with an alphanumeric character"))
		})
	})

	Context("with multiple validation errors", func() {
		It("should return all validation errors for a name that violates multiple rules", func() {
			// Name that is too long AND has consecutive hyphens
			longNameWithConsecutiveHyphens := strings.Repeat("a", 50) + "--" + strings.Repeat("b", 15)
			obj := createTestObject(longNameWithConsecutiveHyphens)
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(2))

			// Should have both length and consecutive hyphen errors
			fieldDetails := make([]string, len(apiErr.Fields))
			for i, field := range apiErr.Fields {
				fieldDetails[i] = field.Detail
			}
			Expect(fieldDetails).To(ConsistOf(
				"name must be no more than 63 characters",
				"name must not contain consecutive '-' characters",
			))
		})

		It("should return all validation errors for a name that violates regex and consecutive hyphen rules", func() {
			// Name that starts with hyphen AND has consecutive hyphens
			obj := createTestObject("-invalid--name")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(2))

			fieldDetails := make([]string, len(apiErr.Fields))
			for i, field := range apiErr.Fields {
				fieldDetails[i] = field.Detail
			}
			Expect(fieldDetails).To(ConsistOf(
				"name must consist of lower case alphanumeric characters or '-', start and end with an alphanumeric character",
				"name must not contain consecutive '-' characters",
			))
		})
	})

	Context("error message formatting", func() {
		It("should include filename in error detail when present", func() {
			obj := createTestObject("Invalid-Name", "test-file.yaml")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Detail).To(ContainSubstring("defined in file \"test-file.yaml\""))
		})

		It("should not include filename in error detail when not present", func() {
			obj := createTestObject("Invalid-Name")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Detail).NotTo(ContainSubstring("defined in file"))
			Expect(apiErr.Detail).To(Equal("Test failed validation"))
		})

		It("should include object kind and name in error title", func() {
			obj := createTestObject("Invalid-Name") // Use invalid name with uppercase
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Title).To(Equal("Failed to validate object \"Invalid-Name\""))
		})

		It("should set correct error type and status", func() {
			obj := createTestObject("Invalid-Name")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Type).To(Equal("ValidationError"))
			Expect(apiErr.Status).To(Equal(400))
		})
	})

	Context("edge cases", func() {
		It("should handle names with only hyphens", func() {
			obj := createTestObject("---")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(2))

			fieldDetails := make([]string, len(apiErr.Fields))
			for i, field := range apiErr.Fields {
				fieldDetails[i] = field.Detail
			}
			Expect(fieldDetails).To(ConsistOf(
				"name must consist of lower case alphanumeric characters or '-', start and end with an alphanumeric character",
				"name must not contain consecutive '-' characters",
			))
		})

		It("should handle names with mixed case and consecutive hyphens", func() {
			obj := createTestObject("Invalid--Name")
			err := common.ValidateObjectName(obj)

			Expect(err).To(HaveOccurred())

			apiErr, ok := common.AsApiError(err)
			Expect(ok).To(BeTrue())
			Expect(apiErr.Fields).To(HaveLen(2))
		})
	})
})
