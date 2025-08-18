// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identifier

import (
	"testing"
)

func TestConvertFileIdToPath(t *testing.T) {
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
			got, err := ConvertFileIdToPath(tt.fileId)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertFileIdToPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ConvertFileIdToPath() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConvertPathToFileId(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
		wantErr  bool
	}{
		{
			name:     "Simple valid path",
			path:     "dev/group1/team1/file.txt",
			expected: "dev--group1--team1--file.txt",
			wantErr:  false,
		},
		{
			name:     "Path with subdirectories in filename part",
			path:     "dev/group1/team1/subdir/file.txt",
			expected: "dev--group1--team1--subdir/file.txt",
			wantErr:  false,
		},
		{
			name:     "Invalid  path format",
			path:     "invalid/format",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertPathToFileId(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertPathToFileId() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ConvertPathToFileId() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that converting from fileId to s3Path and back gives the original fileId
	fileId := "dev--group1--team1--complex--file--name.txt"

	s3Path, err := ConvertFileIdToPath(fileId)
	if err != nil {
		t.Fatalf("Failed to convert fileId to s3Path: %v", err)
	}

	roundTripFileId, err := ConvertPathToFileId(s3Path)
	if err != nil {
		t.Fatalf("Failed to convert s3Path back to fileId: %v", err)
	}

	if fileId != roundTripFileId {
		t.Errorf("Round trip conversion failed. Original: %s, Result: %s", fileId, roundTripFileId)
	}
}
