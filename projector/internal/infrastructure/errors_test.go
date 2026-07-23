// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
)

var _ = Describe("IsFKViolation", func() {
	It("should return true for a PostgreSQL foreign key violation (23503)", func() {
		pgErr := &pgconn.PgError{Code: "23503"}
		Expect(infrastructure.IsFKViolation(pgErr, "")).To(BeTrue())
	})

	It("should return true for a wrapped foreign key violation", func() {
		pgErr := &pgconn.PgError{Code: "23503"}
		wrapped := fmt.Errorf("upsert failed: %w", pgErr)
		Expect(infrastructure.IsFKViolation(wrapped, "")).To(BeTrue())
	})

	It("should match a named constraint", func() {
		pgErr := &pgconn.PgError{Code: "23503", ConstraintName: "fk_target"}
		Expect(infrastructure.IsFKViolation(pgErr, "fk_target")).To(BeTrue())
	})

	It("should return false for a different constraint name", func() {
		pgErr := &pgconn.PgError{Code: "23503", ConstraintName: "fk_owner"}
		Expect(infrastructure.IsFKViolation(pgErr, "fk_target")).To(BeFalse())
	})

	It("should return false for a different PostgreSQL error code", func() {
		pgErr := &pgconn.PgError{Code: "23505"} // unique_violation
		Expect(infrastructure.IsFKViolation(pgErr, "")).To(BeFalse())
	})

	It("should return false for a non-PostgreSQL error", func() {
		Expect(infrastructure.IsFKViolation(errors.New("some error"), "")).To(BeFalse())
	})

	It("should return false for nil", func() {
		Expect(infrastructure.IsFKViolation(nil, "")).To(BeFalse())
	})
})
