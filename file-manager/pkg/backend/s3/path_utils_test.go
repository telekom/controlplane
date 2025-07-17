// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"testing"
)

func TestConvertFileIdToS3Path(t *testing.T) {
	tests := []struct {
		name     string
		fileId   string
		expected string
		wantErr  bool
	}{
		{
			name:     "Simple valid fileId",
			fileId:   "dev--group1--team1--file.txt",
			expected: "dev/group1/team1/file.txt",
			wantErr:  false,
		},
		{
			name:     "Complex filename with dashes",
			fileId:   "dev--group1--team1--file--with--dashes.txt",
			expected: "dev/group1/team1/file--with--dashes.txt",
			wantErr:  false,
		},
		{
			name:     "Invalid fileId format",
			fileId:   "invalid-format",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertFileIdToS3Path(tt.fileId)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertFileIdToS3Path() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ConvertFileIdToS3Path() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConvertS3PathToFileId(t *testing.T) {
	tests := []struct {
		name     string
		s3Path   string
		expected string
		wantErr  bool
	}{
		{
			name:     "Simple valid S3 path",
			s3Path:   "dev/group1/team1/file.txt",
			expected: "dev--group1--team1--file.txt",
			wantErr:  false,
		},
		{
			name:     "Path with subdirectories in filename part",
			s3Path:   "dev/group1/team1/subdir/file.txt",
			expected: "dev--group1--team1--subdir/file.txt",
			wantErr:  false,
		},
		{
			name:     "Invalid S3 path format",
			s3Path:   "invalid/format",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertS3PathToFileId(tt.s3Path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertS3PathToFileId() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ConvertS3PathToFileId() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that converting from fileId to s3Path and back gives the original fileId
	fileId := "dev--group1--team1--complex--file--name.txt"

	s3Path, err := ConvertFileIdToS3Path(fileId)
	if err != nil {
		t.Fatalf("Failed to convert fileId to s3Path: %v", err)
	}

	roundTripFileId, err := ConvertS3PathToFileId(s3Path)
	if err != nil {
		t.Fatalf("Failed to convert s3Path back to fileId: %v", err)
	}

	if fileId != roundTripFileId {
		t.Errorf("Round trip conversion failed. Original: %s, Result: %s", fileId, roundTripFileId)
	}
}
