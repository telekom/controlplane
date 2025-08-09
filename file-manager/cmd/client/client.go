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

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
	"github.com/telekom/controlplane/file-manager/api"
	"go.uber.org/zap"

	fp "path/filepath"
)

var (
	url       string
	token     string
	tokenFile string

	fileId         string
	filepath       string
	outputFile     string
	ignoreChecksum bool

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

	fmt.Println("No file operation specified. Use --file to upload or --id to download.")
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
