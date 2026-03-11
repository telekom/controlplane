// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package base_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
)

var _ = Describe("FileCommand", func() {
	var (
		fileCmd  *base.FileCommand
		tempDir  string
		yamlFile string
	)

	BeforeEach(func() {
		var err error
		// Create a temporary directory for test files
		tempDir, err = os.MkdirTemp("", "file-cmd-test")
		Expect(err).NotTo(HaveOccurred())

		// Create a temporary YAML file for testing
		yamlFile = filepath.Join(tempDir, "test.yaml")
		yamlContent := `apiVersion: tcp.ei.telekom.de/v1
kind: Rover
metadata:
  name: test-rover
spec:
  exposures:
    - port: 8080
      security:
        username: testuser
        password: testpass
`

		err = os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		Expect(err).NotTo(HaveOccurred())

		// Create a new file command
		fileCmd = base.NewFileCommand("test", "Test command", "Test command long description")
	})

	AfterEach(func() {
		// Clean up test files
		_ = os.RemoveAll(tempDir)
	})

	Describe("NewFileCommand", func() {
		It("should create a new file command with the provided values", func() {
			// Verify the command properties
			Expect(fileCmd).NotTo(BeNil())
			Expect(fileCmd.BaseCommand).NotTo(BeNil())
			Expect(fileCmd.Cmd).NotTo(BeNil())
			Expect(fileCmd.Cmd.Use).To(Equal("test"))
			Expect(fileCmd.Cmd.Short).To(Equal("Test command"))
			Expect(fileCmd.Cmd.Long).To(Equal("Test command long description"))
		})
	})

	Describe("InitParser", func() {
		It("should initialize the parser", func() {
			// Initialize the parser
			err := fileCmd.InitParser()

			// Verify no error occurred
			Expect(err).NotTo(HaveOccurred())

			// Verify the parser was initialized
			Expect(fileCmd.Parser).NotTo(BeNil())
		})
	})

	Describe("ParseFiles", func() {
		Context("when the parser is not initialized", func() {
			It("should initialize the parser and parse files", func() {
				// Set the file path
				fileCmd.FilePath = yamlFile

				// Parse files
				err := fileCmd.ParseFiles()

				// Verify no error occurred
				Expect(err).NotTo(HaveOccurred())

				// Verify the parser was initialized
				Expect(fileCmd.Parser).NotTo(BeNil())

				// Verify files were parsed
				Expect(fileCmd.Parser.Objects()).To(HaveLen(1))
				Expect(fileCmd.Parser.Objects()[0].GetKind()).To(Equal("Rover"))
				Expect(fileCmd.Parser.Objects()[0].GetName()).To(Equal("test-rover"))
			})
		})

		Context("when the parser is already initialized", func() {
			It("should parse files without reinitializing", func() {
				// Initialize the parser first
				err := fileCmd.InitParser()
				Expect(err).NotTo(HaveOccurred())

				// Set the file path
				fileCmd.FilePath = yamlFile

				// Parse files
				err = fileCmd.ParseFiles()

				// Verify no error occurred
				Expect(err).NotTo(HaveOccurred())

				// Verify files were parsed
				Expect(fileCmd.Parser.Objects()).To(HaveLen(1))
				Expect(fileCmd.Parser.Objects()[0].GetKind()).To(Equal("Rover"))
				Expect(fileCmd.Parser.Objects()[0].GetName()).To(Equal("test-rover"))
			})
		})

		Context("when the file does not exist", func() {
			It("should return an error", func() {
				// Set a non-existent file path
				fileCmd.FilePath = filepath.Join(tempDir, "non-existent.yaml")

				// Try to parse files
				err := fileCmd.ParseFiles()

				// Verify an error occurred
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
