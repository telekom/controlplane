// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package database_test

import (
	"fmt"
	"regexp"
	"strings"

	"entgo.io/ent/entc/gen"

	"github.com/telekom/controlplane/controlplane-api/ent/migrate"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var snakeCase = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

// brokenSplits holds the names that Ent derives for plural initialisms inside
// an identifier, for example "team_APIs" -> "team_ap_is" and
// "external_IDs" -> "external_i_ds".
var brokenSplits = func() []string {
	snake := gen.Funcs["snake"].(func(string) string)
	initialisms := []string{"API", "HTTP", "ID", "IDP", "IP", "JSON", "M2M", "MCP", "SQL", "SSE", "UID", "URI", "URL", "UUID"}
	splits := make([]string, len(initialisms))
	for i, initialism := range initialisms {
		// Ent does not split at the start of a name, so derive the split after a prefix.
		splits[i] = strings.TrimPrefix(snake("x_"+initialism+"s"), "x_")
	}
	return splits
}()

// namingViolation returns a reason if the database identifier is not
// lowercase snake_case or contains a broken initialism split.
func namingViolation(name string) string {
	if !snakeCase.MatchString(name) {
		return "is not lowercase snake_case"
	}
	for _, split := range brokenSplits {
		if strings.Contains("_"+name+"_", "_"+split+"_") {
			return fmt.Sprintf("contains broken initialism split %q; pin the name with entsql.Table or StorageKey", split)
		}
	}
	return ""
}

var _ = Describe("Database identifier naming", func() {
	It("uses clean snake_case names for all tables, columns, foreign keys and indexes", func() {
		// Arrange
		var violations []string
		check := func(kind, name, label string) {
			if reason := namingViolation(name); reason != "" {
				violations = append(violations, fmt.Sprintf("%s %s %s", kind, label, reason))
			}
		}

		// Act
		for _, table := range migrate.Tables {
			check("table", table.Name, table.Name)
			for _, column := range table.Columns {
				check("column", column.Name, table.Name+"."+column.Name)
			}
			for _, fk := range table.ForeignKeys {
				check("foreign key", fk.Symbol, fk.Symbol)
			}
			for _, index := range table.Indexes {
				check("index", index.Name, index.Name)
			}
		}

		// Assert
		Expect(violations).To(BeEmpty(), strings.Join(violations, "\n"))
	})

	DescribeTable("flags broken identifiers",
		func(name string) {
			// Arrange and act
			reason := namingViolation(name)

			// Assert
			Expect(reason).NotTo(BeEmpty())
		},
		Entry("table from APIs", "ap_is"),
		Entry("foreign-key column from APIs", "team_ap_is"),
		Entry("column from IDs", "external_i_ds"),
		Entry("mixed-case foreign-key symbol", "api_exposures_applications_exposed_APIs"),
	)

	DescribeTable("accepts clean identifiers",
		func(name string) {
			// Arrange and act
			reason := namingViolation(name)

			// Assert
			Expect(reason).To(BeEmpty())
		},
		Entry("pinned table", "apis"),
		Entry("pinned foreign-key column", "team_apis"),
		Entry("pinned column", "external_ids"),
		Entry("similar word", "cap_is_set"),
	)
})
