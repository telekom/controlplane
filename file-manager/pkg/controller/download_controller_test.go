// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller_test

import (
	"bytes"
	"context"
	"io"

	"github.com/telekom/controlplane/file-manager/pkg/controller"
	"github.com/telekom/controlplane/file-manager/test/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	XFileContentType = "X-File-Content-Type"
	XFileChecksum    = "X-File-Checksum"
)

var _ = Describe("DownloadController", func() {
	Context("Download controller", func() {
		mockedBackend := &mocks.MockFileDownloader{}

		It("Should download files", func() {
			ctx := context.Background()
			ctrl := controller.NewDownloadController(mockedBackend)

			writer := getMockWriterWithContent("this is a test file content")

			headers := make(map[string]string)
			headers[XFileContentType] = "application/yaml"
			headers[XFileChecksum] = "thisIsATestChecksum"

			mockedBackend.EXPECT().DownloadFile(any(ctx), "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60").Return(writer, headers, nil)

			file, m, err := ctrl.DownloadFile(ctx, "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60")

			// check contents
			if buf, ok := (file).(*bytes.Buffer); ok {
				Expect(buf.String()).To(Equal("this is a test file content"))
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(file).To(Equal(writer))

			By("checking the returned headers")
			Expect(m).To(HaveKeyWithValue("X-File-Content-Type", "application/yaml"))
			Expect(m).To(HaveKeyWithValue("X-File-Checksum", "thisIsATestChecksum"))
		})

		It("Should pass legacy file IDs through to the backend during migration", func() {
			ctx := context.Background()
			ctrl := controller.NewDownloadController(mockedBackend)

			legacyFileId := "poc--eni--hyperion--my-test-file"
			writer := getMockWriterWithContent("legacy file contents")
			mockedBackend.EXPECT().DownloadFile(any(ctx), "poc/eni/hyperion/my-test-file").Return(writer, map[string]string{}, nil).Once()

			file, _, err := ctrl.DownloadFile(ctx, legacyFileId)
			Expect(err).NotTo(HaveOccurred())
			Expect(file).To(Equal(writer))
		})
	})
})

func getMockWriterWithContent(content string) io.ReadWriter {
	return bytes.NewBuffer([]byte(content))
}
