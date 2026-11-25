// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package files

import (
	"errors"
	"regexp"
	"strings"
)

const (
	maxFileIDLength = 512
	legacyIDParts   = 4
)

var (
	uuidPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	legacyPattern = regexp.MustCompile(`^[a-z0-9.-]+$`)
)

// ValidateFileID accepts a canonical lowercase UUID or a legacy ID of at
// least four non-empty parts separated by "--" (lowercase letters, digits,
// '-' and '.' only, no ".."). Everything else is rejected before any upstream
// call is made.
func ValidateFileID(id string) error {
	if id == "" {
		return errors.New("file id must not be empty")
	}
	if len(id) > maxFileIDLength {
		return errors.New("file id is too long")
	}
	if uuidPattern.MatchString(id) {
		return nil
	}
	if !legacyPattern.MatchString(id) || strings.Contains(id, "..") {
		return errors.New("file id contains invalid characters")
	}
	parts := strings.Split(id, "--")
	if len(parts) < legacyIDParts {
		return errors.New("file id has an unsupported format")
	}
	for _, p := range parts {
		if p == "" {
			return errors.New("file id has an empty segment")
		}
	}
	return nil
}
