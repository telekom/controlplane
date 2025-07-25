// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identifier

import (
	"strings"

	"github.com/pkg/errors"
)

// ConvertFileIdToPath converts a fileId in the format "<env>--<group>--<team>--<fileName>"
// to a path with virtual folders: "<env>/<group>/<team>/<fileName>"
// This allows for better organization and facilitates browsing/management
func ConvertFileIdToPath(fileId string) (string, error) {
	// Parse the fileId using the controller utility
	parts, err := ParseFileID(fileId)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse fileId")
	}

	// Build the path with slashes
	s3Path := parts.Env + "/" + parts.Group + "/" + parts.Team + "/" + parts.FileName

	return s3Path, nil
}

// ConvertPathToFileId converts a path with virtual folders back to a fileId
// This is the reverse operation of ConvertFileIdToPath
func ConvertPathToFileId(s3Path string) (string, error) {
	// Split the path into parts
	parts := strings.SplitN(s3Path, "/", 4)
	if len(parts) != 4 {
		return "", errors.New("invalid S3 path format, expected <env>/<group>/<team>/<fileName>")
	}

	// Extract the parts
	env := parts[0]
	group := parts[1]
	team := parts[2]
	fileName := parts[3]

	// Build the fileId with --
	fileId := env + "--" + group + "--" + team + "--" + fileName

	return fileId, nil
}
