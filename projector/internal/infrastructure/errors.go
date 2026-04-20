// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package infrastructure

import "errors"

// ErrEntityNotFound is returned when an IDResolver lookup finds no matching
// entity in either the cache or the database.
var ErrEntityNotFound = errors.New("entity not found")
