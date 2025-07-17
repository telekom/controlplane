// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/pkg/controller"
	"strings"
)

// ConvertFileIdToS3Path converts a fileId in the format "<env>--<group>--<team>--<fileName>"
// to an S3 path with virtual folders: "<env>/<group>/<team>/<fileName>"
// This allows for better organization in S3 and facilitates browsing/management
func ConvertFileIdToS3Path(fileId string) (string, error) {
	// Parse the fileId using the controller utility
	parts, err := controller.ParseFileID(fileId)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse fileId")
	}

	// Build the S3 path with slashes
	s3Path := parts.Env + "/" + parts.Group + "/" + parts.Team + "/" + parts.FileName

	return s3Path, nil
}

// ConvertS3PathToFileId converts an S3 path with virtual folders back to a fileId
// This is the reverse operation of ConvertFileIdToS3Path
func ConvertS3PathToFileId(s3Path string) (string, error) {
	// Split the S3 path into parts
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
