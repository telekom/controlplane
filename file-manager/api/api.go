// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/api/accesstoken"
	"github.com/telekom/controlplane/common-server/api/util"
	"github.com/telekom/controlplane/file-manager/api/gen"
	"github.com/telekom/controlplane/file-manager/pkg/constants"
	"io"
	"net/http"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
	"sync"
	"github.com/telekom/controlplane/secret-manager/api/util"
)

const (
	localhost                = "http://localhost:9090/api"
	inCluster                = "https://file-manager.file-manager-system.svc.cluster.local/api"
	tokenFilePath            = "/var/run/secrets/tokens/sa-token"
	uploadRequestContentType = "application/octet-stream"
)

var (
	ErrNotFound = errors.New("resource not found")
	once        sync.Once
	api         FileManager
)

type FileContentType string

var (
	FileContentTypeJSON   FileContentType = "application/json"
	FileContentTypeYaml   FileContentType = "application/yaml"
	FileContentTypeBinary FileContentType = "application/octet-stream"
)

type DownloadApi interface {
	DownloadFile(ctx context.Context, fileId string) (*io.ReadCloser, error)
}

type UploadApi interface {
	UploadFile(ctx context.Context, fileId string, fileContentType FileContentType, content *io.Reader) (*FileUploadResponse, error)
}

type FileManager interface {
	UploadApi
	DownloadApi
}

var _ FileManager = (*fileManagerAPI)(nil)

type fileManagerAPI struct {
	client gen.ClientWithResponsesInterface
}

type Options struct {
	URL           string
	Token         accesstoken.AccessToken
	SkipTLSVerify bool
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
	if util.IsRunningInCluster() {
		return &Options{
			URL:   inCluster,
			Token: accesstoken.NewAccessToken(tokenFilePath),
		}
	} else {
		return &Options{
			URL:   localhost,
			Token: nil,
		}
	}
}

type Option func(*Options)

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
		util.NewHttpClientOrDie(
			util.WithClientName("file-manager"),
			util.WithReplacePattern(`^\/api\/v1\/files\/(?P<redacted>.*)$`),
			util.WithSkipTlsVerify(skipTlsVerify),
			util.WithCaFilepath(""),
		)),
		gen.WithRequestEditorFn(options.accessTokenReqEditor))

	if err != nil {
		panic(fmt.Sprintf("Failed to create client: %v", err))
	}
	return &fileManagerAPI{
		client: httpClient,
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

func (f *fileManagerAPI) UploadFile(ctx context.Context, fileId string, fileContentType FileContentType, content *io.Reader) (*FileUploadResponse, error) {
	log := log.FromContext(ctx)
	log.V(1).Info("Uploading file ", "fileId", fileId, "fileContentType", fileContentType)

	// calculate the MD5 hash
	data, err := io.ReadAll(*content)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to read content of file while calculating MD5 hash")
	}

	md5hash, err := Md5Base64(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to calculate MD5 hash")
	}

	// use generated client code to call the file manager server
	response, err := f.client.UploadFileWithBodyWithResponse(ctx, fileId, &gen.UploadFileParams{
		XFileContentType: fileContentTypeAsStringPtr(fileContentType),
		XFileChecksum:    stringPtr(md5hash),
	}, uploadRequestContentType, io.NopCloser(bytes.NewReader(data)))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to upload file")
	}

	// evaluate the response
	switch response.StatusCode() {
	case http.StatusOK:
		return &FileUploadResponse{
			MD5Hash:     extractHeader(response, constants.HeaderNameChecksum),
			FileId:      response.JSON200.Id,
			ContentType: extractHeader(response, constants.HeaderNameOriginalContentType),
		}, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		var err gen.ErrorResponse
		if err := json.Unmarshal(response.Body, &err); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("error %s: %s", err.Type, err.Detail)
	}
}

func (f *fileManagerAPI) DownloadFile(ctx context.Context, fileId string) (*io.ReadCloser, error) {
	log := log.FromContext(ctx)
	response, err := f.client.DownloadFileWithResponse(ctx, fileId)
	if err != nil {
		return nil, err
	}
	switch response.StatusCode() {
	case http.StatusOK:
		bodyReadCloser := io.NopCloser(bytes.NewReader(response.Body))
		defer func(bodyReadCloser io.ReadCloser) {
			err := bodyReadCloser.Close()
			if err != nil {
				log.Error(err, "failed to close response body")
			}
		}(bodyReadCloser)

		return &bodyReadCloser, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		var err gen.ErrorResponse
		if err := json.Unmarshal(response.Body, &err); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("error %s: %s", err.Type, err.Detail)
	}
}
