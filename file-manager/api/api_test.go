// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/file-manager/api/gen"
	"github.com/telekom/controlplane/file-manager/pkg/constants"
	"github.com/telekom/controlplane/file-manager/test/mocks"
)

func TestUploadFile(t *testing.T) {
	type fields struct {
		mockResp *gen.UploadFileResponse
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
				mockResp: &gen.UploadFileResponse{
					HTTPResponse: &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							string(constants.HeaderNameChecksum):            []string{"abc123"},
							string(constants.HeaderNameOriginalContentType): []string{"application/pdf"},
						},
					},
					JSON200: &gen.FileUploadResponse{
						Id: "file123",
					},
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
				mockResp: &gen.UploadFileResponse{
					HTTPResponse: &http.Response{
						StatusCode: http.StatusNotFound,
					},
				},
			},
			fileID:      "missing-file",
			contentType: "application/pdf",
			content:     []byte("hello"),
			wantErr:     true,
			expectedErr: ErrNotFound,
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
			mockClient := &mocks.MockClientWithResponsesInterface{}
			mockClient.On("UploadFileWithBodyWithResponse", mock.Anything, tt.fileID, mock.Anything, "application/octet-stream", mock.Anything).
				Return(tt.fields.mockResp, tt.fields.mockErr)

			f := &fileManagerAPI{
				client: mockClient,
			}

			r := io.Reader(bytes.NewReader(tt.content))
			resp, err := f.UploadFile(context.TODO(), tt.fileID, tt.contentType, &r)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr.Error(), err.Error())
				}
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedFileID, resp.FileId)
				assert.Equal(t, tt.expectedMD5, resp.MD5Hash)
				assert.Equal(t, tt.contentType, resp.ContentType)
			}
		})
	}
}

func TestDownloadFile(t *testing.T) {
	type fields struct {
		mockResp *gen.DownloadFileResponse
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
				mockResp: &gen.DownloadFileResponse{
					HTTPResponse: &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							string(constants.HeaderNameChecksum):            []string{"abc123"},
							string(constants.HeaderNameOriginalContentType): []string{"application/yaml"},
						},
					},
					Body: []byte("file content"),
				},
			},
			fileID:                    "file123",
			wantContent:               "file content",
			wantErr:                   false,
			expectedChecksumHeader:    "abc123",
			expectedContentTypeHeader: "application/yaml",
		},
		{
			name: "not found",
			fields: fields{
				mockResp: &gen.DownloadFileResponse{
					HTTPResponse: &http.Response{
						StatusCode: http.StatusNotFound,
					},
					Body: nil,
				},
			},
			fileID:      "missing",
			wantErr:     true,
			expectedErr: ErrNotFound,
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
			mockClient := &mocks.MockClientWithResponsesInterface{}
			mockClient.On("DownloadFileWithResponse", mock.Anything, tt.fileID).
				Return(tt.fields.mockResp, tt.fields.mockErr)

			f := &fileManagerAPI{
				client: mockClient,
			}

			resp, err := f.DownloadFile(context.TODO(), tt.fileID)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr.Error(), err.Error())
				}
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantContent, string(resp.Content))
				assert.Equal(t, tt.expectedContentTypeHeader, resp.ContentType)
				assert.Equal(t, tt.expectedChecksumHeader, resp.MD5Hash)
			}
		})
	}
}
