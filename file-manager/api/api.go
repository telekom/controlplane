// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/client"
	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
	"github.com/telekom/controlplane/common-server/pkg/util"
	"github.com/telekom/controlplane/file-manager/api/constants"
	"github.com/telekom/controlplane/file-manager/api/gen"
)

const (
	localhost                = "http://localhost:8443/api"
	inCluster                = "https://file-manager.file-manager-system.svc.cluster.local/api"
	TokenFilePath            = "/var/run/secrets/filemgr/token"
	uploadRequestContentType = "application/octet-stream"
)

var (
	ErrNotFound = errors.New("resource not found")
	once        sync.Once
	api         FileManager
)

type DownloadApi interface {
	DownloadFile(ctx context.Context, fileId string, w io.Writer) (*FileDownloadResponse, error)
}

type UploadApi interface {
	UploadFile(ctx context.Context, fileId string, fileContentType string, r io.Reader) (*FileUploadResponse, error)
}

type FileManager interface {
	UploadApi
	DownloadApi
}

var _ FileManager = (*FileManagerAPI)(nil)

type FileManagerAPI struct {
	Client           gen.ClientInterface
	ValidateChecksum bool
}

type Options struct {
	URL              string
	Token            accesstoken.AccessToken
	SkipTLSVerify    bool
	ValidateChecksum bool
}

func (o *Options) accessTokenReqEditor(ctx context.Context, req *http.Request) error {
	if o.Token == nil {
		return nil
	}
	token, err := o.Token.Read()
	if err != nil {
		return errors.Wrap(err, "failed to read access token")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return nil
}

func defaultOptions() *Options {
	opts := &Options{
		ValidateChecksum: true,
	}
	if util.IsRunningInCluster() {
		opts.URL = inCluster
		opts.Token = accesstoken.NewAccessToken(TokenFilePath)
	} else {
		opts.URL = localhost
	}

	return opts
}

type Option func(*Options)

func WithURL(url string) Option {
	return func(o *Options) {
		o.URL = url
	}
}
func WithAccessToken(token accesstoken.AccessToken) Option {
	return func(o *Options) {
		o.Token = token
	}
}

func WithSkipTLSVerify(skip bool) Option {
	return func(o *Options) {
		o.SkipTLSVerify = skip
	}
}

func WithValidateChecksum(validate bool) Option {
	return func(o *Options) {
		o.ValidateChecksum = validate
	}
}

func New(opts ...Option) FileManager {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	if !strings.HasPrefix(options.URL, "https://") {
		fmt.Println("⚠️\tWarning: Using HTTP instead of HTTPS. This is not secure.")
	}
	skipTlsVerify := os.Getenv("SKIP_TLS_VERIFY") == "true" || options.SkipTLSVerify

	httpClient, err := gen.NewClientWithResponses(options.URL, gen.WithHTTPClient(
		client.NewHttpClientOrDie(
			client.WithClientName("file-manager"),
			client.WithReplacePattern(`^\/api\/v1\/files\/(?P<redacted>.*)$`),
			client.WithSkipTlsVerify(skipTlsVerify),
			client.WithCaFilepath(""),
		)),
		gen.WithRequestEditorFn(options.accessTokenReqEditor))

	if err != nil {
		log.Fatalf("Failed to create file manager client: %v", err)
	}
	return &FileManagerAPI{
		Client:           httpClient,
		ValidateChecksum: options.ValidateChecksum,
	}
}

func GetFileManager(opts ...Option) FileManager {
	if api == nil {
		once.Do(func() {
			api = New(opts...)
		})
	}
	return api
}

func (f *FileManagerAPI) UploadFile(ctx context.Context, fileId string, fileContentType string, r io.Reader) (*FileUploadResponse, error) {
	log := logr.FromContextOrDiscard(ctx)

	buf := bytes.NewBuffer(nil)
	size, hash, err := copyAndHash(buf, r)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to copy content")
	}
	log.V(1).Info("Uploading file ", "fileId", fileId, "fileContentType", fileContentType, "size", size, "hash", hash)

	params := &gen.UploadFileParams{
		XFileContentType: &fileContentType,
	}
	if f.ValidateChecksum {
		params.XFileChecksum = &hash
	}
	// use generated client code to call the file manager server
	response, err := f.Client.UploadFileWithBody(ctx, fileId, params, uploadRequestContentType, buf)

	if err != nil {
		return nil, errors.Wrap(err, "Failed to upload file")
	}
	defer response.Body.Close() //nolint:errcheck

	// evaluate the response
	switch response.StatusCode {
	case http.StatusOK:
		var res gen.FileUploadResponse
		if err := json.NewDecoder(response.Body).Decode(&res); err != nil {
			return nil, errors.Wrap(err, "failed to decode response body")
		}
		checksum := extractHeader(response, constants.XFileChecksum)
		if f.ValidateChecksum && checksum != hash {
			return nil, errors.Errorf("checksum mismatch: expected %s, got %s", checksum, hash)
		}
		return &FileUploadResponse{
			FileHash:    checksum,
			FileId:      res.Id,
			ContentType: extractHeader(response, constants.XFileContentType),
		}, nil

	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		var err gen.ErrorResponse
		if err := json.NewDecoder(response.Body).Decode(&err); err != nil {
			return nil, errors.Wrap(err, "failed to decode error response")
		}
		return nil, errors.Errorf("error %s: %s", err.Type, err.Detail)
	}
}

func (f *FileManagerAPI) DownloadFile(ctx context.Context, fileId string, w io.Writer) (*FileDownloadResponse, error) {
	log := logr.FromContextOrDiscard(ctx)
	response, err := f.Client.DownloadFile(ctx, fileId)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close() //nolint:errcheck

	switch response.StatusCode {
	case http.StatusOK:
		size, hash, err := copyAndHash(w, response.Body)
		if err != nil {
			return nil, errors.Wrap(err, "failed to copy file content")
		}
		log.V(1).Info("Downloaded file", "fileId", fileId, "size", size, "hash", hash)

		expectedChecksum := extractHeader(response, constants.XFileChecksum)
		if f.ValidateChecksum && hash != expectedChecksum {
			return nil, errors.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, hash)
		}
		return &FileDownloadResponse{
			FileHash:    expectedChecksum,
			ContentType: extractHeader(response, constants.XFileContentType),
		}, nil

	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		var err gen.ErrorResponse
		if err := json.NewDecoder(response.Body).Decode(&err); err != nil {
			return nil, errors.Wrap(err, "failed to decode error response")
		}
		return nil, errors.Errorf("error %s: %s", err.Type, err.Detail)
	}
}
