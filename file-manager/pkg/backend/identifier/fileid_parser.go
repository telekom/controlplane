// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identifier

import (
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// ParseFileID parses a fileId that must be a canonical (lowercase, hyphenated) UUID
func ParseFileID(fileId string) (uuid.UUID, error) {
	id, err := uuid.Parse(fileId)
	if err != nil {
		return uuid.Nil, errors.Wrap(err, "invalid fileId format, expected UUID")
	}
	// uuid.Parse also accepts urn:uuid:, braced and uppercase forms which would map to different object keys.
	if id.String() != fileId {
		return uuid.Nil, errors.New("invalid fileId format, expected canonical lowercase UUID")
	}
	return id, nil
}

// ValidateFileID returns nil if the fileId is a valid canonical UUID
func ValidateFileID(fileId string) error {
	_, err := ParseFileID(fileId)
	return err
}
