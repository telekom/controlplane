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
// PostgreSQL foreign-key constraint violation (SQLSTATE 23503).
// This is used to detect races where a cached FK ID points to a row that
// no longer exists in the database.
func IsFKViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503"
	}
	return false
}
