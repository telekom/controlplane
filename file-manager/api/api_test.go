// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/file-manager/api/constants"
	gen_test "github.com/telekom/controlplane/file-manager/api/gen/mock"
)

func TestUploadFile(t *testing.T) {
	type fields struct {
		mockResp *http.Response
		mockErr  error
	}

	tests := []struct {
		name           string
		fields         fields
		fileID         string
		contentType    string
		content        []byte
		wantErr        bool
		expectedErr    error
		expectedFileID string
		expectedMD5    string
	}{
		{
			name: "success",
			fields: fields{
				mockResp: &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						constants.XFileChecksum:    []string{"abc123"},
						constants.XFileContentType: []string{"application/pdf"},
					},
					Body: io.NopCloser(strings.NewReader(`{"id": "file123"}`)),
				},
			},
			fileID:         "file123",
			contentType:    "application/pdf",
			content:        []byte("hello"),
			wantErr:        false,
			expectedFileID: "file123",
			expectedMD5:    "abc123",
		},
		{
			name: "not found",
			fields: fields{
				mockResp: &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(nil),
				},
			},
			fileID:      "missing-file",
			contentType: "application/pdf",
			content:     []byte("hello"),
			wantErr:     true,
			expectedErr: api.ErrNotFound,
		},
		{
			name: "http error",
			fields: fields{
				mockResp: nil,
				mockErr:  errors.New("network error"),
			},
			fileID:      "file123",
			contentType: "application/pdf",
			content:     []byte("hello"),
			wantErr:     true,
			expectedErr: errors.New("Failed to upload file: network error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &gen_test.MockClientInterface{}
			mockClient.On("UploadFileWithBody", mock.Anything, tt.fileID, mock.Anything, "application/octet-stream", mock.Anything).
				Return(tt.fields.mockResp, tt.fields.mockErr)

			f := &api.FileManagerAPI{
				Client: mockClient,
			}

			r := bytes.NewReader(tt.content)
			resp, err := f.UploadFile(context.TODO(), tt.fileID, tt.contentType, r)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr.Error(), err.Error())
				}
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedFileID, resp.FileId)
				assert.Equal(t, tt.expectedMD5, resp.FileHash)
				assert.Equal(t, tt.contentType, resp.ContentType)
			}
		})
	}
}

func TestDownloadFile(t *testing.T) {
	type fields struct {
		mockResp *http.Response
		mockErr  error
	}
	tests := []struct {
		name                      string
		fields                    fields
		fileID                    string
		wantContent               string
		expectedContentTypeHeader string
		expectedChecksumHeader    string
		wantErr                   bool
		expectedErr               error
	}{
		{
			name: "success",
			fields: fields{
				mockResp: &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						constants.XFileChecksum:    []string{"0QtMP/Ejsm3AaNQ6i+8tIw=="},
						constants.XFileContentType: []string{"application/yaml"},
					},
					Body: io.NopCloser(strings.NewReader("file content")),
				},
			},
			fileID:                    "file123",
			wantContent:               "file content",
			wantErr:                   false,
			expectedChecksumHeader:    "0QtMP/Ejsm3AaNQ6i+8tIw==",
			expectedContentTypeHeader: "application/yaml",
		},
		{
			name: "not found",
			fields: fields{
				mockResp: &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(nil),
				},
			},
			fileID:      "missing",
			wantErr:     true,
			expectedErr: api.ErrNotFound,
		},
		{
			name: "http error",
			fields: fields{
				mockErr: errors.New("timeout"),
			},
			fileID:      "file123",
			wantErr:     true,
			expectedErr: errors.New("timeout"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &gen_test.MockClientInterface{}
			mockClient.On("DownloadFile", mock.Anything, tt.fileID).
				Return(tt.fields.mockResp, tt.fields.mockErr)

			f := &api.FileManagerAPI{
				Client: mockClient,
			}

			buf := bytes.NewBuffer(nil)

			resp, err := f.DownloadFile(context.TODO(), tt.fileID, buf)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr.Error(), err.Error())
				}
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantContent, buf.String())
				assert.Equal(t, tt.expectedContentTypeHeader, resp.ContentType)
				assert.Equal(t, tt.expectedChecksumHeader, resp.FileHash)
			}
		})
	}
}
