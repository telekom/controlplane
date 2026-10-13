// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/telekom/controlplane/file-manager/api/constants"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestCopyAndMeasure(t *testing.T) {
	r, n, err := copyAndMeasure(strings.NewReader("hello"))
	assert.NoError(t, err)
	assert.Equal(t, int64(5), n)
	b, _ := io.ReadAll(r)
	assert.Equal(t, "hello", string(b))

	_, _, err = copyAndMeasure(failingReader{})
	assert.Error(t, err)
}

func TestPrepareMetadata(t *testing.T) {
	uploader := NewBucketFileUploader(&BucketConfig{})

	in := map[string]string{
		constants.XFileContentType: "application/yaml",
		constants.XFileChecksum:    "abc",
	}
	userMetadata, contentType := uploader.prepareMetadata(context.Background(), in)
	assert.Equal(t, "application/yaml", contentType)
	assert.Equal(t, in, userMetadata)

	userMetadata, contentType = uploader.prepareMetadata(context.Background(), nil)
	assert.Equal(t, constants.DefaultContentType, contentType)
	assert.Empty(t, userMetadata)
}
