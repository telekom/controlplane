// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identifier

import (
	"strings"

	"github.com/pkg/errors"
)

// FileIDParts represents the individual components of a file ID
type FileIDParts struct {
	Env      string
	Group    string
	Team     string
	FileName string
	Raw      string // The original raw fileId
}

// ParseFileID parses a fileId in the format "<env>--<group>--<team>--<fileName>" into its components
// Returns the parsed parts and an error if the format is invalid
func ParseFileID(fileId string) (*FileIDParts, error) {
	// Split the key by --
	parts := strings.SplitN(fileId, "--", 4)
	if len(parts) != 4 {
		return nil, errors.New("invalid fileId format, expected <env>--<group>--<team>--<fileName>")
	}

	// Extract the parts
	env := parts[0]
	group := parts[1]
	team := parts[2]
	fileName := parts[3]

	// Validate non-empty parts
	if env == "" || group == "" || team == "" || fileName == "" {
		return nil, errors.New("all parts of key must be non-empty")
	}

	return &FileIDParts{
		Env:      env,
		Group:    group,
		Team:     team,
		FileName: fileName,
		Raw:      fileId,
	}, nil
}

// ValidateFileID validates if the fileId is in the correct format without returning the parts
// Returns nil if valid, error if invalid
func ValidateFileID(fileId string) error {
	_, err := ParseFileID(fileId)
	return err
}
