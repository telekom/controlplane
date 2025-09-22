// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/test"
	"github.com/telekom/controlplane/common/pkg/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// MockNamedObject is a simple implementation of types.NamedObject for testing
type MockNamedObject struct {
	Name      string
	Namespace string
}

func (m *MockNamedObject) GetName() string {
	return m.Name
}

func (m *MockNamedObject) GetNamespace() string {
	return m.Namespace
}

func (m *MockNamedObject) GetGenerateName() string {
	return ""
}

var _ types.NamedObject = &MockNamedObject{}

var _ = Describe("ValidationError", func() {

	var (
		valErr    *ValidationError
		testObj   *MockNamedObject
		groupKind schema.GroupKind
	)

	BeforeEach(func() {
		testObj = &MockNamedObject{
			Name:      "test-object",
			Namespace: "test-namespace",
		}
		groupKind = schema.GroupKind{Group: "test.group", Kind: "TestKind"}
		valErr = NewValidationError(groupKind, testObj)
		Expect(valErr).NotTo(BeNil())
	})

	Context("NewValidationError", func() {
		It("should initialize with empty errors and warnings", func() {
			Expect(valErr.HasErrors()).To(BeFalse())
			Expect(valErr.BuildWarnings()).To(BeNil())
			Expect(valErr.BuildError()).To(BeNil())

			// Verify internal state
			Expect(valErr.Errors).To(BeEmpty())
			Expect(valErr.Warnings).To(BeEmpty())
			Expect(valErr.gk).To(Equal(groupKind))
			Expect(valErr.ref).To(Equal(testObj))
		})
	})

	Context("AddNewError", func() {
		It("should add an error to the error list", func() {
			path := field.NewPath("spec").Child("field")
			value := "invalid-value"
			message := "field is invalid"

			// Add the error
			valErr.AddInvalidError(path, value, message)

			// Verify error was added
			Expect(valErr.HasErrors()).To(BeTrue())
			Expect(valErr.Errors).To(HaveLen(1))
			Expect(valErr.Errors[0].Type).To(Equal(field.ErrorTypeInvalid))
			Expect(valErr.Errors[0].Field).To(Equal("spec.field"))
			Expect(valErr.Errors[0].BadValue).To(Equal(value))
			Expect(valErr.Errors[0].Detail).To(Equal(message))

			// Add another error
			path2 := field.NewPath("metadata").Child("name")
			value2 := ""
			message2 := "name cannot be empty"
			valErr.AddInvalidError(path2, value2, message2)

			// Verify both errors exist
			Expect(valErr.Errors).To(HaveLen(2))
			Expect(valErr.Errors[1].Field).To(Equal("metadata.name"))
		})
	})

	Context("BuildError", func() {
		It("should return nil when there are no errors", func() {
			Expect(valErr.BuildError()).To(BeNil())
		})

		It("should build a properly formatted StatusError when there are errors", func() {
			// Add test errors
			valErr.AddInvalidError(field.NewPath("spec").Child("field1"), "invalid-value", "field1 is invalid")
			valErr.AddInvalidError(field.NewPath("spec").Child("field2"), "/invalid", "field2 is invalid")

			// Build the error
			statusErr := valErr.BuildError().(*apierrors.StatusError)
			Expect(statusErr).NotTo(BeNil())

			// Verify error details
			Expect(statusErr.ErrStatus.Status).To(Equal(metav1.StatusFailure))
			Expect(statusErr.ErrStatus.Message).To(ContainSubstring("TestKind"))
			Expect(statusErr.ErrStatus.Message).To(ContainSubstring("test-object"))
			Expect(statusErr.ErrStatus.Reason).To(Equal(metav1.StatusReasonInvalid))
			Expect(statusErr.ErrStatus.Details.Name).To(Equal("test-object"))
			Expect(statusErr.ErrStatus.Details.Group).To(Equal(groupKind.Group))
			Expect(statusErr.ErrStatus.Details.Kind).To(Equal(groupKind.Kind))
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(2))

			// Verify first cause
			Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.field1"))
			Expect(statusErr.ErrStatus.Details.Causes[0].Message).To(ContainSubstring("field1 is invalid"))

			// Verify second cause
			Expect(statusErr.ErrStatus.Details.Causes[1].Field).To(Equal("spec.field2"))
			Expect(statusErr.ErrStatus.Details.Causes[1].Message).To(ContainSubstring("field2 is invalid"))
		})

		It("should use error type as is", func() {
			// Add errors with different types
			path1 := field.NewPath("spec").Child("required")
			valErr.Errors = append(valErr.Errors, field.Required(path1, "required field is missing"))

			path2 := field.NewPath("spec").Child("duplicate")
			valErr.Errors = append(valErr.Errors, field.Duplicate(path2, "duplicate value"))

			// Build the error
			statusErr := valErr.BuildError().(*apierrors.StatusError)
			Expect(statusErr).NotTo(BeNil())

			// Verify types are preserved
			Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(2))
			// Kubernetes uses string types for CauseType, not the field.ErrorType constants directly
			Expect(statusErr.ErrStatus.Details.Causes[0].Type).To(Equal(metav1.CauseTypeFieldValueRequired))
			Expect(statusErr.ErrStatus.Details.Causes[1].Type).To(Equal(metav1.CauseTypeFieldValueDuplicate))
		})
	})

	Context("Warnings", func() {
		It("should add and build warnings correctly", func() {
			// Add warnings
			warning1 := "This is a test warning"
			valErr.AddWarning(warning1)

			path := field.NewPath("spec").Child("field")
			value := "test-value"
			format := "Invalid value %q for %s"
			valErr.AddWarningf(path, value, format, value, "field")

			// Verify warnings
			warnings := valErr.BuildWarnings()
			Expect(warnings).To(HaveLen(2))
			Expect(warnings[0]).To(Equal(warning1))
			Expect(warnings[1]).To(ContainSubstring("spec.field"))
			Expect(warnings[1]).To(ContainSubstring("test-value"))
		})

		It("should return nil when there are no warnings", func() {
			Expect(valErr.BuildWarnings()).To(BeNil())
		})

		It("should add warnings with formatted messages", func() {
			format := "Value %d exceeds maximum allowed value of %d"
			path := field.NewPath("spec").Child("count")
			value := 200
			maxValue := 100

			valErr.AddWarningf(path, value, format, value, maxValue)

			warnings := valErr.BuildWarnings()
			Expect(warnings).To(HaveLen(1))

			expectedWarning := fmt.Sprintf("spec.count: %s", fmt.Sprintf(format, value, maxValue))
			Expect(warnings[0]).To(Equal(expectedWarning))
		})
	})

	Context("Integration with Kubernetes objects", func() {
		It("should work correctly with Kubernetes objects", func() {
			// Create a real Kubernetes object
			obj := &test.TestResource{
				TypeMeta: metav1.TypeMeta{
					Kind:       "TestResource",
					APIVersion: "test.group/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-object",
					Namespace: "default",
				},
			}

			// Create a validation error with it
			gk := schema.GroupKind{Group: "test.group", Kind: "TestResource"}
			valErr := NewValidationError(gk, obj)

			// Add some errors
			valErr.AddInvalidError(field.NewPath("spec").Child("zone"), "", "zone is required")

			// Build error and verify
			statusErr := valErr.BuildError().(*apierrors.StatusError)
			Expect(statusErr).NotTo(BeNil())

			Expect(statusErr.ErrStatus.Details.Name).To(Equal("test-object"))
			Expect(statusErr.ErrStatus.Details.Kind).To(Equal("TestResource"))
			Expect(statusErr.ErrStatus.Details.Group).To(Equal("test.group"))
		})
	})
})
