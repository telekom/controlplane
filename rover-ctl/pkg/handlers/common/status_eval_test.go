// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"bytes"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ = Describe("StatusEval", func() {
	var (
		obj    *types.UnstructuredObject
		status *common.ObjectStatusResponse
		eval   common.StatusEval
	)

	BeforeEach(func() {
		obj = &types.UnstructuredObject{
			Content: map[string]any{
				"apiVersion": "v1",
				"kind":       "Test",
				"metadata": map[string]any{
					"name": "test-resource",
				},
			},
		}

		// Create a status object with various states for testing
		status = &common.ObjectStatusResponse{
			ProcessingState: "done",
			OverallStatus:   "complete",
			Errors:          []types.StatusInfo{},
			Warnings:        []types.StatusInfo{},
			Info:            []types.StatusInfo{},
		}

		eval = common.NewStatusEval(obj, status)
	})

	Describe("IsSuccess", func() {
		Context("when status is complete and processing is done", func() {
			It("should return true", func() {
				status.OverallStatus = "complete"
				status.ProcessingState = "done"
				Expect(eval.IsSuccess()).To(BeTrue())
			})
		})

		Context("when overall status is not complete", func() {
			It("should return false", func() {
				status.OverallStatus = "pending"
				status.ProcessingState = "done"
				Expect(eval.IsSuccess()).To(BeFalse())
			})
		})

		Context("when processing state is not done", func() {
			It("should return false", func() {
				status.OverallStatus = "complete"
				status.ProcessingState = "processing"
				Expect(eval.IsSuccess()).To(BeFalse())
			})
		})
	})

	Describe("IsFailure", func() {
		Context("when overall status is failed", func() {
			It("should return true", func() {
				status.OverallStatus = "failed"
				Expect(eval.IsFailure()).To(BeTrue())
			})
		})

		Context("when overall status is not failed", func() {
			It("should return false", func() {
				status.OverallStatus = "complete"
				Expect(eval.IsFailure()).To(BeFalse())
			})
		})
	})

	Describe("IsBlocked", func() {
		Context("when overall status is blocked", func() {
			It("should return true", func() {
				status.OverallStatus = "blocked"
				Expect(eval.IsBlocked()).To(BeTrue())
			})
		})

		Context("when overall status is not blocked", func() {
			It("should return false", func() {
				status.OverallStatus = "complete"
				Expect(eval.IsBlocked()).To(BeFalse())
			})
		})
	})

	Describe("IsProcessed", func() {
		Context("when processing state is done", func() {
			It("should return true", func() {
				status.ProcessingState = "done"
				Expect(eval.IsProcessed()).To(BeTrue())
			})
		})

		Context("when processing state is not done", func() {
			It("should return false", func() {
				status.ProcessingState = "processing"
				Expect(eval.IsProcessed()).To(BeFalse())
			})
		})
	})

	Describe("PrettyPrint", func() {
		Context("when format is console", func() {
			It("should call ConsolePrettyPrint with expected output", func() {
				var buf bytes.Buffer
				err := eval.PrettyPrint(&buf, "console")
				Expect(err).NotTo(HaveOccurred())

				// Verify output contains expected elements for console format
				output := buf.String()
				Expect(output).To(ContainSubstring("Resource: Test/test-resource"))
				Expect(output).To(ContainSubstring("Status: complete | Processing: done"))
				Expect(output).To(ContainSubstring("✅ Operation completed successfully"))
			})
		})

		Context("when format is json", func() {
			It("should call JsonPrettyPrint with valid JSON output", func() {
				var buf bytes.Buffer
				err := eval.PrettyPrint(&buf, "json")
				Expect(err).NotTo(HaveOccurred())

				// Verify output is valid JSON with expected structure
				output := buf.String()
				var jsonOutput map[string]any
				err = json.Unmarshal([]byte(output), &jsonOutput)
				Expect(err).NotTo(HaveOccurred())

				Expect(jsonOutput).To(HaveKeyWithValue("resource", "Test/test-resource"))
				Expect(jsonOutput).To(HaveKeyWithValue("status", "complete"))
				Expect(jsonOutput).To(HaveKeyWithValue("processingState", "done"))
				Expect(jsonOutput).To(HaveKeyWithValue("success", true))
			})
		})

		Context("when format is unknown", func() {
			It("should return an error", func() {
				var buf bytes.Buffer
				err := eval.PrettyPrint(&buf, "unknown")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("unknown format: unknown"))
			})
		})
	})

	Describe("ConsolePrettyPrint", func() {
		Context("when there are errors", func() {
			It("should include error information in the output", func() {
				status.Errors = []types.StatusInfo{
					{
						Cause:   "InvalidInput",
						Message: "Invalid resource configuration",
						Details: "Field 'spec.replicas' must be greater than 0",
						Resource: types.ObjectRef{
							ApiVersion: "v1",
							Kind:       "TestResource",
							Name:       "test-error",
						},
					},
				}

				var buf bytes.Buffer
				err := eval.ConsolePrettyPrint(&buf)
				Expect(err).NotTo(HaveOccurred())

				output := buf.String()
				Expect(output).To(ContainSubstring("❌ Errors"))
				Expect(output).To(ContainSubstring("Kind: TestResource"))
				Expect(output).To(ContainSubstring("Resource: test-error"))
				Expect(output).To(ContainSubstring("Cause:   InvalidInput"))
				Expect(output).To(ContainSubstring("Message: Invalid resource configuration"))
				Expect(output).To(ContainSubstring("Details: Field 'spec.replicas' must be greater than 0"))
			})
		})

		Context("when there are warnings", func() {
			It("should include warning information in the output", func() {
				status.Warnings = []types.StatusInfo{
					{
						Cause:   "ResourceLimit",
						Message: "Approaching resource limit",
						Details: "90% of quota used",
						Resource: types.ObjectRef{
							ApiVersion: "v1",
							Kind:       "TestResource",
							Name:       "test-warning",
						},
					},
				}

				var buf bytes.Buffer
				err := eval.ConsolePrettyPrint(&buf)
				Expect(err).NotTo(HaveOccurred())

				output := buf.String()
				Expect(output).To(ContainSubstring("⚠️  Warnings"))
				Expect(output).To(ContainSubstring("Kind: TestResource"))
				Expect(output).To(ContainSubstring("Resource: test-warning"))
				Expect(output).To(ContainSubstring("Cause:   ResourceLimit"))
				Expect(output).To(ContainSubstring("Message: Approaching resource limit"))
				Expect(output).To(ContainSubstring("Details: 90% of quota used"))
			})
		})

		Context("when there is info", func() {
			It("should include info messages in the output", func() {
				status.Info = []types.StatusInfo{
					{
						Cause:   "ResourceCreated",
						Message: "Resource created successfully",
						Resource: types.ObjectRef{
							ApiVersion: "v1",
							Kind:       "TestResource",
							Name:       "test-info",
						},
					},
				}

				var buf bytes.Buffer
				err := eval.ConsolePrettyPrint(&buf)
				Expect(err).NotTo(HaveOccurred())

				output := buf.String()
				Expect(output).To(ContainSubstring("ℹ️  Information"))
				Expect(output).To(ContainSubstring("Kind: TestResource"))
				Expect(output).To(ContainSubstring("Resource: test-info"))
				Expect(output).To(ContainSubstring("Cause:   ResourceCreated"))
				Expect(output).To(ContainSubstring("Message: Resource created successfully"))
			})
		})
	})

	Describe("JsonPrettyPrint", func() {
		Context("when all status types are present", func() {
			It("should produce valid JSON with proper structure", func() {
				// Set up status with errors, warnings and info
				status.Errors = []types.StatusInfo{
					{
						Cause:   "Error1",
						Message: "Error message",
						Resource: types.ObjectRef{
							Kind: "ErrorKind",
							Name: "error-resource",
						},
					},
				}
				status.Warnings = []types.StatusInfo{
					{
						Cause:   "Warning1",
						Message: "Warning message",
						Resource: types.ObjectRef{
							Kind: "WarningKind",
							Name: "warning-resource",
						},
					},
				}
				status.Info = []types.StatusInfo{
					{
						Cause:   "Info1",
						Message: "Info message",
						Resource: types.ObjectRef{
							Kind: "InfoKind",
							Name: "info-resource",
						},
					},
				}

				var buf bytes.Buffer
				err := eval.JsonPrettyPrint(&buf)
				Expect(err).NotTo(HaveOccurred())

				// Parse the JSON output
				var result map[string]any
				err = json.Unmarshal(buf.Bytes(), &result)
				Expect(err).NotTo(HaveOccurred())

				// Verify the structure
				Expect(result).To(HaveKeyWithValue("resource", "Test/test-resource"))
				Expect(result).To(HaveKeyWithValue("status", "complete"))
				Expect(result).To(HaveKeyWithValue("processingState", "done"))
				Expect(result).To(HaveKey("errors"))
				Expect(result).To(HaveKey("warnings"))
				Expect(result).To(HaveKey("info"))

				// Check errors
				errors := result["errors"].(map[string]any)
				Expect(errors).To(HaveKey("ErrorKind"))
				errorList := errors["ErrorKind"].([]any)
				Expect(errorList).To(HaveLen(1))
				errorItem := errorList[0].(map[string]any)
				Expect(errorItem).To(HaveKeyWithValue("cause", "Error1"))

				// Check warnings
				warnings := result["warnings"].(map[string]any)
				Expect(warnings).To(HaveKey("WarningKind"))

				// Check info
				info := result["info"].(map[string]any)
				Expect(info).To(HaveKey("InfoKind"))
			})
		})
	})
})
