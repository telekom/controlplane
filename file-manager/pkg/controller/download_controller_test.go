// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller_test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/file-manager/pkg/controller"
	"github.com/telekom/controlplane/file-manager/test/mocks"

	"io"
)

const (
	XFileContentType = "X-File-Content-Type"
	XFileChecksum    = "X-File-Checksum"
)

var _ = Describe("DownloadController", func() {

	Context("Download controller", func() {

		var mockedBackend = &mocks.MockFileDownloader{}

		It("Should download files", func() {
			ctx := context.Background()
			ctrl := controller.NewDownloadController(mockedBackend)

			var writer *io.Writer
			var headers = make(map[string]string)
			headers[XFileContentType] = "application/yaml"
			headers[XFileChecksum] = "thisIsATestChecksum"

			mockedBackend.EXPECT().DownloadFile(any(ctx), "poc/eni/hyperion/my-test-file").Return(writer, headers, nil)

			file, m, err := ctrl.DownloadFile(ctx, "poc--eni--hyperion--my-test-file")
			Expect(err).NotTo(HaveOccurred())
			Expect(file).To(Equal(writer))

			By("checking the returned headers")
			Expect(m).To(HaveKeyWithValue("X-File-Content-Type", "application/yaml"))
			Expect(m).To(HaveKeyWithValue("X-File-Checksum", "thisIsATestChecksum"))
		})

		It("Should not download files with wrong fileId format - complete nonsense", func() {
			ctx := context.Background()
			ctrl := controller.NewDownloadController(mockedBackend)

			file, m, err := ctrl.DownloadFile(ctx, "obviously_wrong/id")
			Expect(err).To(HaveOccurred())
			By("returning the correct error message")
			Expect(err.Error()).To(BeEquivalentTo("InvalidFileId: invalid file ID 'obviously_wrong/id'"))
			Expect(m).To(BeNil())
			Expect(file).To(BeNil())
		})

		It("Should not download files with wrong fileId format - wrong number of parts", func() {
			ctx := context.Background()
			ctrl := controller.NewDownloadController(mockedBackend)

			file, m, err := ctrl.DownloadFile(ctx, "poc--eni--fileId")
			Expect(err).To(HaveOccurred())
			By("returning the correct error message")
			Expect(err.Error()).To(BeEquivalentTo("InvalidFileId: invalid file ID 'poc--eni--fileId'"))
			Expect(m).To(BeNil())
			Expect(file).To(BeNil())
		})

	})

})
