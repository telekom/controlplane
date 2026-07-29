// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrEntityNotFound is returned when an IDResolver lookup finds no matching
// entity in either the cache or the database.
var ErrEntityNotFound = errors.New("entity not found")

// IsFKViolation reports whether err (or any error in its chain) is a
// PostgreSQL foreign-key constraint violation (SQLSTATE 23503) on the named
// constraint. This is used to detect races where a cached FK ID points to a
// row that no longer exists in the database. Passing an empty constraint
// matches any FK violation.
func IsFKViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503" && (constraint == "" || pgErr.ConstraintName == constraint)
	}
	return false
}
