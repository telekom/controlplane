// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GraphQL naming contract", func() {
	var schema *ast.Schema
	wrongInitialism := regexp.MustCompile(`(^|[a-z0-9])(Api|Mcp|Ip|Idp|Id|Http|Url|Sse|M2m)(s([A-Z]|$)|[A-Z]|$)`)

	BeforeEach(func() {
		// Arrange
		_, thisFile, _, ok := runtime.Caller(0)
		Expect(ok).To(BeTrue())
		root := filepath.Join(filepath.Dir(thisFile), "..", "..")
		sources := []*ast.Source{
			{Name: "ent.graphql", Input: readSchemaFile(filepath.Join(root, "ent.graphql"))},
			{Name: "schema.graphql", Input: readSchemaFile(filepath.Join(root, "schema.graphql"))},
			{Name: "mutation.graphql", Input: readSchemaFile(filepath.Join(root, "mutation.graphql"))},
		}

		// Act
		var err error
		schema, err = gqlparser.LoadSchema(sources...)

		// Assert
		Expect(err).NotTo(HaveOccurred())
	})

	It("uses standard casing for all public names", func() {
		// Arrange
		pascalCase := regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
		camelCase := regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
		enumValue := regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)*$`)

		// Act
		var violations []string
		for name, definition := range schema.Types {
			if strings.HasPrefix(name, "__") {
				continue
			}
			if !pascalCase.MatchString(name) {
				violations = append(violations, fmt.Sprintf("type %s is not PascalCase", name))
			}
			for _, field := range definition.Fields {
				if strings.HasPrefix(field.Name, "__") {
					continue
				}
				if !camelCase.MatchString(field.Name) {
					violations = append(violations, fmt.Sprintf("field %s.%s is not camelCase", name, field.Name))
				}
				for _, argument := range field.Arguments {
					if !camelCase.MatchString(argument.Name) {
						violations = append(violations, fmt.Sprintf("argument %s.%s(%s) is not camelCase", name, field.Name, argument.Name))
					}
				}
			}
			for _, value := range definition.EnumValues {
				if !enumValue.MatchString(value.Name) {
					violations = append(violations, fmt.Sprintf("enum value %s.%s is not SCREAMING_SNAKE_CASE", name, value.Name))
				}
			}
		}
		for name := range schema.Directives {
			if !camelCase.MatchString(name) {
				violations = append(violations, fmt.Sprintf("directive %s is not camelCase", name))
			}
		}

		// Assert
		Expect(violations).To(BeEmpty(), strings.Join(violations, "\n"))
	})

	It("uses Go-style initialisms in public names", func() {
		// Arrange
		var violations []string

		// Act
		for name, definition := range schema.Types {
			if strings.HasPrefix(name, "__") {
				continue
			}
			if wrongInitialism.MatchString(name) {
				violations = append(violations, "type "+name)
			}
			for _, field := range definition.Fields {
				if wrongInitialism.MatchString(field.Name) {
					violations = append(violations, fmt.Sprintf("field %s.%s", name, field.Name))
				}
				for _, argument := range field.Arguments {
					if wrongInitialism.MatchString(argument.Name) {
						violations = append(violations, fmt.Sprintf("argument %s.%s(%s)", name, field.Name, argument.Name))
					}
				}
			}
		}

		// Assert
		Expect(violations).To(BeEmpty(), strings.Join(violations, "\n"))
	})

	DescribeTable("detects mis-cased initialisms without flagging whole words",
		func(name string, invalid bool) {
			// Arrange and act
			actual := wrongInitialism.MatchString(name)

			// Assert
			Expect(actual).To(Equal(invalid))
		},
		Entry("type", "ApiExposure", true),
		Entry("embedded plural", "externalIds", true),
		Entry("predicate", "hasMcpServersWith", true),
		Entry("URL", "gatewayUrlNEQ", true),
		Entry("HTTP", "enforceGetHttpRequest", true),
		Entry("correct", "hasMCPServersWith", false),
		Entry("initial lowercase word", "apiCategories", false),
		Entry("ordinary word", "Rapid", false),
	)
})

func readSchemaFile(path string) string {
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	return string(data)
}
