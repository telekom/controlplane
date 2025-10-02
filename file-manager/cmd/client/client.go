// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	fp "path/filepath"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
	"github.com/telekom/controlplane/file-manager/api"
)

var (
	url       string
	token     string
	tokenFile string

	fileId         string
	filepath       string
	outputFile     string
	ignoreChecksum bool
	deleteFile     bool

	fileManager api.FileManager
)

func init() {
	flag.StringVar(&url, "url", "", "API URL")
	flag.StringVar(&token, "token", "", "API Token")
	flag.StringVar(&tokenFile, "token-file", "", "API Token File")

	flag.StringVar(&fileId, "id", "", "File ID")
	flag.StringVar(&filepath, "file", "", "File Path")
	flag.StringVar(&outputFile, "output", "", "Output File Path (for download)")
	flag.BoolVar(&ignoreChecksum, "no-checksum", false, "Ignore checksum validation")
	flag.BoolVar(&deleteFile, "delete", false, "Delete file")
}

func main() {
	flag.Parse()
	log := zapr.NewLogger(zap.Must(zap.NewDevelopment()))
	ctx := logr.NewContext(context.Background(), log)

	opts := []api.Option{
		api.WithSkipTLSVerify(true),
	}
	if url != "" {
		opts = append(opts, api.WithURL(url))
	}
	if token != "" {
		opts = append(opts, api.WithAccessToken(accesstoken.NewStaticAccessToken(token)))
	} else if tokenFile != "" {
		opts = append(opts, api.WithAccessToken(accesstoken.NewAccessToken(tokenFile)))
	}

	if ignoreChecksum {
		opts = append(opts, api.WithValidateChecksum(false))
	}

	fileManager = api.New(opts...)

	if deleteFile && fileId != "" {
		// delete file
		err := fileManager.DeleteFile(ctx, fileId)
		if err != nil {
			// Handle 404 as success with a log message
			if errors.Is(err, api.ErrNotFound) {
				log.Info("File not found (already deleted or never existed), treating as success", "fileId", fileId)
				fmt.Println("File not found (already deleted or never existed):", fileId)
				return
			}
			// All other errors are actual errors
			panic(err)
		}
		fmt.Println("File deleted successfully:", fileId)
		return
	}

	if filepath != "" && fileId != "" {
		// upload file
		file, err := os.Open(filepath)
		if err != nil {
			fmt.Println("Error opening file:", err)
			return
		}
		defer file.Close() //nolint:errcheck

		contentType, err := detectContentType(file)
		if err != nil {
			contentType = "application/octet-stream" // Fallback content type
		}

		resp, err := fileManager.UploadFile(ctx, fileId, contentType, file)
		if err != nil {
			panic(err)
		}
		fmt.Println("File uploaded successfully:", resp)

		return
	}

	if fileId != "" {
		// download file
		w := os.Stdout
		if outputFile != "" {
			file, err := os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				panic(err)
			}
			defer file.Close() //nolint:errcheck
			w = file
		}
		fileInfo, err := fileManager.DownloadFile(ctx, fileId, w)
		if err != nil {
			panic(err)
		}
		fmt.Printf("File Info: %+v\n", fileInfo)
		return
	}

	fmt.Println("No file operation specified. Use --file to upload, --id to download or --id and --delete to delete.")
}

func detectContentType(file *os.File) (string, error) {
	ext := fp.Ext(file.Name())
	contentType := mime.TypeByExtension(ext)
	if contentType != "" {
		return contentType, nil
	}
	buffer := make([]byte, 512)
	_, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}
	file.Seek(0, 0) //nolint:errcheck

	contentType = http.DetectContentType(buffer)
	return contentType, nil
}
