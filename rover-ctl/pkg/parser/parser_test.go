// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/parser"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ = Describe("Parser", func() {
	// Test for removeUnknownObjects function
	Describe("removeUnknownObjects", func() {
		It("should remove objects without kind or apiVersion", func() {
			// Create a new parser
			objParser := parser.NewObjectParser()

			// Parse a file with a mix of valid and invalid objects
			err := objParser.Parse(filepath.Join("testdata", "unknown-objects.yaml"))
			Expect(err).NotTo(HaveOccurred())

			filteredObjects := objParser.Objects()

			// We should have only 1 object after filtering (the valid one)
			Expect(filteredObjects).To(HaveLen(1))
			Expect(filteredObjects[0].GetKind()).To(Equal("Rover"))
			Expect(filteredObjects[0].GetApiVersion()).To(Equal("tcp.ei.telekom.de/v1"))
			Expect(filteredObjects[0].GetName()).To(Equal("valid-object"))
		})
	})

	var (
		testdataDir  string
		objectParser *parser.ObjectParser
	)

	BeforeEach(func() {
		// Get the absolute path to the testdata directory
		testdataDir = filepath.Join("testdata")
		// Create a new parser without validation by default
		objectParser = parser.NewObjectParser()
	})

	Context("Parse", func() {
		It("should return an error when parsing an empty path", func() {
			// Attempt to parse an empty path
			err := objectParser.Parse("")

			// Verify an error occurred
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("path cannot be empty"))
		})

		It("should return an error when parsing a non-existent file", func() {
			// Attempt to parse a non-existent file
			err := objectParser.Parse(filepath.Join(testdataDir, "non-existent-file.yaml"))

			// Verify an error occurred
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("file not found"))
		})

		It("should successfully parse a valid YAML file", func() {
			// Parse a valid YAML file
			err := objectParser.Parse(filepath.Join(testdataDir, "valid-resource.yaml"))

			// Verify no error occurred
			Expect(err).NotTo(HaveOccurred())

			// Verify the parser contains the parsed object
			objects := objectParser.Objects()
			Expect(objects).To(HaveLen(1))
			Expect(objects[0].GetKind()).To(Equal("Rover"))
			Expect(objects[0].GetApiVersion()).To(Equal("tcp.ei.telekom.de/v1"))
			Expect(objects[0].GetName()).To(Equal("test-rover"))
		})

		It("should return an error when parsing a file with an unsupported extension", func() {
			// Attempt to parse a file with an unsupported extension
			err := objectParser.Parse(filepath.Join(testdataDir, "unsupported-file.txt"))

			// Verify an error occurred
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported file extension"))
		})

		It("should return an error when parsing an invalid JSON file", func() {
			// Attempt to parse an invalid JSON file
			err := objectParser.Parse(filepath.Join(testdataDir, "invalid-json.json"))

			// Verify an error occurred
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse JSON"))
		})

		It("should successfully parse a valid JSON file", func() {
			// Create a new parser with the custom hooks needed for proper JSON parsing
			jsonParser := parser.NewObjectParser(parser.Opts...)

			validFilesDir := filepath.Join(testdataDir, "subdir")
			err := jsonParser.Parse(filepath.Join(validFilesDir, "valid-resource.json"))

			// Verify no error occurred
			Expect(err).NotTo(HaveOccurred())

			// Verify the parser contains the parsed object
			objects := jsonParser.Objects()
			Expect(objects).To(HaveLen(1))
			Expect(objects[0].GetKind()).To(Equal("Rover"))
			Expect(objects[0].GetApiVersion()).To(Equal("tcp.ei.telekom.de/v1"))
			Expect(objects[0].GetName()).To(Equal("test-json-rover"))
		})

		It("should parse multiple documents from a YAML file", func() {
			// Parse a YAML file with multiple documents
			err := objectParser.Parse(filepath.Join(testdataDir, "multi-document.yaml"))

			// Verify no error occurred
			Expect(err).NotTo(HaveOccurred())

			// Verify the parser contains both parsed objects
			objects := objectParser.Objects()
			Expect(objects).To(HaveLen(2))
			Expect(objects[0].GetName()).To(Equal("test-rover-1"))
			Expect(objects[1].GetName()).To(Equal("test-rover-2"))
		})
	})

	Context("Directory Parsing", func() {
		It("should successfully parse all valid files in a directory", func() {
			// Create a new parser just for this test
			dirParser := parser.NewObjectParser()

			validFilesDir := filepath.Join(testdataDir, "subdir")

			// Parse the directory directly
			err := dirParser.Parse(validFilesDir)
			Expect(err).NotTo(HaveOccurred())

			objects := dirParser.Objects()
			Expect(objects).NotTo(BeEmpty())
			Expect(len(objects)).To(Equal(4), "Should find exactly 4 valid objects in the directory")
		})

		It("should return an error when parsing a directory with no valid files", func() {
			// Create a temporary empty directory
			tempDir, err := os.MkdirTemp("", "parser-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tempDir)

			// Attempt to parse the empty directory
			err = objectParser.Parse(tempDir)

			// Verify an error occurred
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no valid YAML or JSON files found in directory"))
		})
	})

	Context("Options", func() {
		It("should create a parser with validation enabled", func() {
			// Create a parser with validation enabled
			validatingParser := parser.NewObjectParser(parser.EnableValidation())

			// Verify the parser was created
			Expect(validatingParser).NotTo(BeNil())
		})

		It("should apply hooks when provided", func() {
			var hookCalled bool
			testHook := func(obj types.Object) error {
				hookCalled = true
				// Modify the object in the hook
				obj.SetProperty("hook-applied", true)
				return nil
			}

			// Create a parser with a test hook
			hookParser := parser.NewObjectParser(
				parser.WithHook(parser.HookAfterParse, testHook),
			)

			// Parse a valid file
			err := hookParser.Parse(filepath.Join(testdataDir, "valid-resource.yaml"))
			Expect(err).NotTo(HaveOccurred())

			// Verify the hook was called and modified the object
			Expect(hookCalled).To(BeTrue())
			Objects := hookParser.Objects()
			Expect(Objects).To(HaveLen(1))
			Expect(Objects[0].GetProperty("hook-applied")).To(Equal(true))
		})
	})

	Context("ApiSpec Parsing", func() {
		It("should correctly parse OpenAPI specs", func() {
			// Create parser with the predefined hooks that handle ApiSpecification
			apiParser := parser.NewObjectParser(parser.Opts...)

			// Parse the OpenAPI spec
			err := apiParser.Parse(filepath.Join(testdataDir, "openapi-spec.yaml"))
			Expect(err).NotTo(HaveOccurred())

			// Verify the parser correctly identified the file as an ApiSpecification
			objects := apiParser.Objects()
			Expect(objects).To(HaveLen(1))
			Expect(objects[0].GetKind()).To(Equal("ApiSpecification"))
			Expect(objects[0].GetApiVersion()).To(Equal("tcp.ei.telekom.de/v1"))
			// The name should be derived from the servers section
			Expect(objects[0].GetName()).To(Equal("v1"))
		})

		It("should correctly parse Swagger specs", func() {
			// Create parser with the predefined hooks that handle ApiSpecification
			apiParser := parser.NewObjectParser(parser.Opts...)

			// Parse the Swagger spec
			err := apiParser.Parse(filepath.Join(testdataDir, "swagger-spec.yaml"))
			Expect(err).NotTo(HaveOccurred())

			// Verify the parser correctly identified the file as an ApiSpecification
			objects := apiParser.Objects()
			Expect(objects).To(HaveLen(1))
			Expect(objects[0].GetKind()).To(Equal("ApiSpecification"))
			Expect(objects[0].GetApiVersion()).To(Equal("tcp.ei.telekom.de/v1"))
			// The name should be derived from the basePath
			Expect(objects[0].GetName()).To(Equal("v2"))
		})
	})

	Context("Iterate", func() {
		It("should iterate through all objects", func() {
			// Parse a file with multiple documents
			err := objectParser.Parse(filepath.Join(testdataDir, "multi-document.yaml"))
			Expect(err).NotTo(HaveOccurred())

			// Count objects using the Iterate method
			count := 0
			for range objectParser.Iterate() {
				count++
			}

			// Verify that the count matches the number of objects
			Expect(count).To(Equal(len(objectParser.Objects())))
		})
	})

	Context("FilterByKindAndVersion", func() {
		It("should filter objects by kind and apiVersion", func() {
			// Parse a file with multiple documents
			err := objectParser.Parse(filepath.Join(testdataDir, "multi-document.yaml"))
			Expect(err).NotTo(HaveOccurred())

			// Filter objects by kind and apiVersion
			filtered := parser.FilterByKindAndVersion(objectParser.Objects(), "Rover", "tcp.ei.telekom.de/v1")

			// Verify that the filtered list contains only the matching objects
			Expect(filtered).To(HaveLen(2))
			for _, obj := range filtered {
				Expect(obj.GetKind()).To(Equal("Rover"))
				Expect(obj.GetApiVersion()).To(Equal("tcp.ei.telekom.de/v1"))
			}
		})
	})
})
