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
	}{
		{name: "Valid UUIDv7", fileId: "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60"},
		{name: "Valid UUIDv4", fileId: "6f1c2d3e-4b5a-4c6d-8e7f-9a0b1c2d3e4f"},
		{name: "Invalid - legacy format", fileId: "dev--groupA--teamB--document.pdf", wantErr: true},
		{name: "Invalid - uppercase UUID", fileId: "01926A3E-7B2C-7D3E-8F4A-1B2C3D4E5F60", wantErr: true},
		{name: "Invalid - urn form", fileId: "urn:uuid:01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60", wantErr: true},
		{name: "Invalid - braced form", fileId: "{01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60}", wantErr: true},
		{name: "Invalid - path traversal", fileId: "../01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60", wantErr: true},
		{name: "Invalid - empty string", fileId: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFileID(tt.fileId)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseFileID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.String() != tt.fileId {
				t.Errorf("ParseFileID() = %v, want %v", got, tt.fileId)
			}
		})
	}
}

func TestValidateFileID(t *testing.T) {
	if err := ValidateFileID("01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60"); err != nil {
		t.Errorf("ValidateFileID() unexpected error = %v", err)
	}
	if err := ValidateFileID("dev--group--team--file.txt"); err == nil {
		t.Error("ValidateFileID() expected error for legacy fileId")
	}
}
