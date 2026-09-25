// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package database_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	entschema "entgo.io/ent/dialect/sql/schema"

	"github.com/telekom/controlplane/controlplane-api/ent/migrate"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// schemaSnapshotPath holds the expected database identifiers. The projector
// migrates with drop-column and drop-index enabled, so a renamed identifier
// drops the old column or index and loses its data.
var schemaSnapshotPath = filepath.Join("testdata", "schema.golden")

// Set UPDATE_SCHEMA_SNAPSHOT=1 to rewrite the snapshot after an intended
// database schema change.
const updateSchemaSnapshotEnv = "UPDATE_SCHEMA_SNAPSHOT"

var _ = Describe("Database schema snapshot", func() {
	It("matches the committed database identifiers", func() {
		// Arrange
		actual := renderSchema(migrate.Tables)
		if os.Getenv(updateSchemaSnapshotEnv) == "1" {
			Expect(os.MkdirAll(filepath.Dir(schemaSnapshotPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(schemaSnapshotPath, []byte(actual), 0o600)).To(Succeed())
		}

		// Act
		expected, err := os.ReadFile(schemaSnapshotPath)

		// Assert
		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(Equal(string(expected)),
			"database schema changed; if intended, rerun with %s=1 and review the snapshot diff", updateSchemaSnapshotEnv)
	})
})

// renderSchema writes tables, columns, foreign keys and indexes in a stable,
// sorted text form. Column order is ignored because it does not affect
// existing databases.
func renderSchema(tables []*entschema.Table) string {
	sorted := slices.Clone(tables)
	slices.SortFunc(sorted, func(a, b *entschema.Table) int { return strings.Compare(a.Name, b.Name) })

	var sb strings.Builder
	for _, t := range sorted {
		fmt.Fprintf(&sb, "table %s\n", t.Name)

		lines := make([]string, 0, len(t.Columns)+len(t.ForeignKeys)+len(t.Indexes))
		for _, c := range t.Columns {
			line := fmt.Sprintf("  column %s %s nullable=%t unique=%t", c.Name, c.Type, c.Nullable, c.Unique)
			if c.Size != 0 {
				line += fmt.Sprintf(" size=%d", c.Size)
			}
			if c.Default != nil {
				line += fmt.Sprintf(" default=%v", c.Default)
			}
			if len(c.Enums) > 0 {
				line += " enums=" + strings.Join(c.Enums, ",")
			}
			lines = append(lines, line)
		}
		for _, fk := range t.ForeignKeys {
			lines = append(lines, fmt.Sprintf("  fk %s (%s) -> %s (%s) on_delete=%s",
				fk.Symbol, columnNames(fk.Columns), fk.RefTable.Name, columnNames(fk.RefColumns), fk.OnDelete))
		}
		for _, idx := range t.Indexes {
			lines = append(lines, fmt.Sprintf("  index %s (%s) unique=%t", idx.Name, columnNames(idx.Columns), idx.Unique))
		}
		slices.Sort(lines)
		for _, line := range lines {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func columnNames(columns []*entschema.Column) string {
	names := make([]string, len(columns))
	for i, c := range columns {
		names[i] = c.Name
	}
	return strings.Join(names, ",")
}
