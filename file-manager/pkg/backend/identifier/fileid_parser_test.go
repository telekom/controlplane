// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identifier

import (
	"testing"
)

func TestParseFileID(t *testing.T) {
	tests := []struct {
		name    string
		fileId  string
		wantErr bool
		want    *FileIDParts
	}{
		{
			name:    "Valid file ID with simple filename",
			fileId:  "dev--groupA--teamB--document.pdf",
			wantErr: false,
			want: &FileIDParts{
				Env:      "dev",
				Group:    "groupA",
				Team:     "teamB",
				FileName: "document.pdf",
				Raw:      "dev--groupA--teamB--document.pdf",
			},
		},
		{
			name:    "Valid file ID with dashes in filename",
			fileId:  "dev--groupA--teamB--my-document-with-dashes.pdf",
			wantErr: false,
			want: &FileIDParts{
				Env:      "dev",
				Group:    "groupA",
				Team:     "teamB",
				FileName: "my-document-with-dashes.pdf",
				Raw:      "dev--groupA--teamB--my-document-with-dashes.pdf",
			},
		},
		{
			name:    "Valid file ID with double dash in filename",
			fileId:  "dev--groupA--teamB--file--with--doubleDash.pdf",
			wantErr: false,
			want: &FileIDParts{
				Env:      "dev",
				Group:    "groupA",
				Team:     "teamB",
				FileName: "file--with--doubleDash.pdf",
				Raw:      "dev--groupA--teamB--file--with--doubleDash.pdf",
			},
		},
		{
			name:    "Invalid file ID - missing parts",
			fileId:  "dev--group--file.txt",
			wantErr: true,
			want:    nil,
		},
		{
			name:    "Invalid file ID - empty string",
			fileId:  "",
			wantErr: true,
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFileID(tt.fileId)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFileID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Skip further checks if we expected an error
			if tt.wantErr {
				return
			}

			// Check fields
			if got.Env != tt.want.Env {
				t.Errorf("ParseFileID() Env = %v, want %v", got.Env, tt.want.Env)
			}
			if got.Group != tt.want.Group {
				t.Errorf("ParseFileID() Group = %v, want %v", got.Group, tt.want.Group)
			}
			if got.Team != tt.want.Team {
				t.Errorf("ParseFileID() Team = %v, want %v", got.Team, tt.want.Team)
			}
			if got.FileName != tt.want.FileName {
				t.Errorf("ParseFileID() FileName = %v, want %v", got.FileName, tt.want.FileName)
			}
			if got.Raw != tt.want.Raw {
				t.Errorf("ParseFileID() Raw = %v, want %v", got.Raw, tt.want.Raw)
			}
		})
	}
}

func TestValidateFileID(t *testing.T) {
	tests := []struct {
		name    string
		fileId  string
		wantErr bool
	}{
		{
			name:    "Valid file ID",
			fileId:  "dev--groupA--teamB--document.pdf",
			wantErr: false,
		},
		{
			name:    "Valid file ID with complex filename",
			fileId:  "dev--groupA--teamB--document--with--dashes.pdf",
			wantErr: false,
		},
		{
			name:    "Invalid file ID - too few parts",
			fileId:  "dev--group--file.txt",
			wantErr: true,
		},
		{
			name:    "Invalid file ID - empty",
			fileId:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileID(tt.fileId)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFileID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
