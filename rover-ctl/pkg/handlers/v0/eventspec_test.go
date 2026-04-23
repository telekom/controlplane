// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	v0 "github.com/telekom/controlplane/rover-ctl/pkg/handlers/v0"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EventSpec Handler", func() {
	Describe("PatchEventSpecificationRequest", func() {
		Context("when spec has a JSON string specification", func() {
			It("should parse the JSON string into a map", func() {
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
						"metadata": map[string]any{
							"name": "test-event",
						},
						"spec": map[string]any{
							"specification": `{"type":"object","properties":{"name":{"type":"string"}}}`,
						},
					},
				}

				err := v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).NotTo(HaveOccurred())

				spec := obj.GetContent()
				specification := spec["specification"]
				Expect(specification).To(BeAssignableToTypeOf(map[string]any{}))

				specMap := specification.(map[string]any)
				Expect(specMap["type"]).To(Equal("object"))

				Expect(obj.GetContent()["apiVersion"]).To(BeNil())
				Expect(obj.GetContent()["kind"]).To(BeNil())
				Expect(obj.GetContent()["metadata"]).To(BeNil())
			})
		})

		Context("when spec has a map specification", func() {
			It("should keep it as-is", func() {
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
						"metadata": map[string]any{
							"name": "test-event",
						},
						"spec": map[string]any{
							"specification": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"name": map[string]any{"type": "string"},
								},
							},
						},
					},
				}

				err := v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).NotTo(HaveOccurred())

				spec := obj.GetContent()
				specMap := spec["specification"].(map[string]any)
				Expect(specMap["type"]).To(Equal("object"))
			})
		})

		Context("when spec is missing", func() {
			It("should return an error", func() {
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
					},
				}

				err := v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Missing 'spec'"))
			})
		})

		Context("when spec is not a map", func() {
			It("should return an error", func() {
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
						"spec":       "invalid",
					},
				}

				err := v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("'spec' should be an object"))
			})
		})

		Context("when specification is an invalid JSON string", func() {
			It("should return a parse error", func() {
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
						"metadata":   map[string]any{"name": "test"},
						"spec": map[string]any{
							"specification": "{invalid json",
						},
					},
				}

				err := v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse JSON schema"))
			})
		})

		Context("when specification is an unsupported type", func() {
			It("should return an error", func() {
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
						"metadata":   map[string]any{"name": "test"},
						"spec": map[string]any{
							"specification": 42,
						},
					},
				}

				err := v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("should be a JSON string or an object"))
			})
		})

		Context("when specification has no specification field", func() {
			It("should succeed without modifying the spec", func() {
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
						"metadata":   map[string]any{"name": "test"},
						"spec": map[string]any{
							"eventType": "test.event",
						},
					},
				}

				err := v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).NotTo(HaveOccurred())

				spec := obj.GetContent()
				Expect(spec["eventType"]).To(Equal("test.event"))
			})
		})

		Context("when specification has a file:// $ref", func() {
			It("should resolve the file reference", func() {
				// Create a temporary JSON schema file
				tmpDir := GinkgoT().TempDir()
				schemaContent := map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{"type": "integer"},
					},
				}
				schemaBytes, err := json.Marshal(schemaContent)
				Expect(err).NotTo(HaveOccurred())

				schemaPath := filepath.Join(tmpDir, "schema.json")
				err = os.WriteFile(schemaPath, schemaBytes, 0o644)
				Expect(err).NotTo(HaveOccurred())

				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
						"metadata":   map[string]any{"name": "test"},
						"spec": map[string]any{
							"specification": `{"$ref":"file://` + schemaPath + `"}`,
						},
					},
				}

				err = v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).NotTo(HaveOccurred())

				spec := obj.GetContent()
				specMap := spec["specification"].(map[string]any)
				Expect(specMap["type"]).To(Equal("object"))
			})
		})

		Context("when specification map has a file:// $ref", func() {
			It("should resolve the file reference from a map specification", func() {
				// Create a temporary JSON schema file
				tmpDir := GinkgoT().TempDir()
				schemaContent := map[string]any{
					"type": "string",
				}
				schemaBytes, err := json.Marshal(schemaContent)
				Expect(err).NotTo(HaveOccurred())

				schemaPath := filepath.Join(tmpDir, "schema.json")
				err = os.WriteFile(schemaPath, schemaBytes, 0o644)
				Expect(err).NotTo(HaveOccurred())

				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
						"metadata":   map[string]any{"name": "test"},
						"spec": map[string]any{
							"specification": map[string]any{
								"$ref": "file://" + schemaPath,
							},
						},
					},
				}

				err = v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).NotTo(HaveOccurred())

				spec := obj.GetContent()
				specMap := spec["specification"].(map[string]any)
				Expect(specMap["type"]).To(Equal("string"))
			})
		})

		Context("when file:// $ref uses a relative path with filename property", func() {
			It("should resolve relative to the resource file directory", func() {
				// Create a temporary directory structure
				tmpDir := GinkgoT().TempDir()

				// Create a subdirectory for schemas
				schemasDir := filepath.Join(tmpDir, "schemas")
				err := os.MkdirAll(schemasDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				schemaContent := map[string]any{"type": "number"}
				schemaBytes, err := json.Marshal(schemaContent)
				Expect(err).NotTo(HaveOccurred())

				err = os.WriteFile(filepath.Join(schemasDir, "event.json"), schemaBytes, 0o644)
				Expect(err).NotTo(HaveOccurred())

				// The resource file is at tmpDir/resource.yaml
				resourceFilePath := filepath.Join(tmpDir, "resource.yaml")

				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
						"metadata":   map[string]any{"name": "test"},
						"spec": map[string]any{
							"specification": map[string]any{
								"$ref": "file://schemas/event.json",
							},
						},
					},
				}
				obj.SetProperty("filename", resourceFilePath)

				err = v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).NotTo(HaveOccurred())

				spec := obj.GetContent()
				specMap := spec["specification"].(map[string]any)
				Expect(specMap["type"]).To(Equal("number"))
			})
		})

		Context("when file:// $ref points to a non-existent file", func() {
			It("should return an error", func() {
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
						"metadata":   map[string]any{"name": "test"},
						"spec": map[string]any{
							"specification": `{"$ref":"file:///nonexistent/path/schema.json"}`,
						},
					},
				}

				err := v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to access JSON schema file"))
			})
		})

		Context("when file:// $ref points to a directory", func() {
			It("should return an error", func() {
				tmpDir := GinkgoT().TempDir()

				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"apiVersion": "tcp.ei.telekom.de/v1",
						"kind":       "EventSpecification",
						"metadata":   map[string]any{"name": "test"},
						"spec": map[string]any{
							"specification": `{"$ref":"file://` + tmpDir + `"}`,
						},
					},
				}

				err := v0.PatchEventSpecificationRequest(context.Background(), obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("directory, expected a file"))
			})
		})
	})

	Describe("NewEventSpecHandlerInstance", func() {
		It("should create an EventSpecHandler with correct configuration", func() {
			handler := v0.NewEventSpecHandlerInstance()
			Expect(handler).NotTo(BeNil())
			Expect(handler.Kind).To(Equal("EventSpecification"))
			Expect(handler.APIVersion).To(Equal("tcp.ei.telekom.de/v1"))
			Expect(handler.Resource).To(Equal("eventspecifications"))
		})
	})
})
