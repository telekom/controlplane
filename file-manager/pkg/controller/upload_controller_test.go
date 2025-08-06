// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller_test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/pkg/controller"
	"github.com/telekom/controlplane/file-manager/test/mocks"
	"strings"

	"io"
)

var _ = Describe("UploadController", func() {

	Context("Upload controller", func() {

		var mockedBackend *mocks.MockFileUploader
		BeforeEach(func() {
			mockedBackend = &mocks.MockFileUploader{}
		})

		It("should upload file successfully", func() {
			ctx := context.Background()
			ctrl := controller.NewUploadController(mockedBackend)

			var reader io.Reader = strings.NewReader("test content")
			var backendMetadata = make(map[string]string)
			backendMetadata["X-File-Content-Type"] = "application/octet-stream"
			backendMetadata["X-File-Checksum"] = "test-checksum"

			mockedBackend.EXPECT().UploadFile(any(ctx), "poc--eni--hyperion--my-test-file", reader, backendMetadata).Return("poc--eni--hyperion--my-test-file", nil)

			var callMetadata = make(map[string]string)
			callMetadata["X-File-Content-Type"] = "application/octet-stream"
			callMetadata["X-File-Checksum"] = "test-checksum"

			file, err := ctrl.UploadFile(ctx, "poc--eni--hyperion--my-test-file", &reader, callMetadata)
			By("returning no error and the same fileId as supplied")
			Expect(err).NotTo(HaveOccurred())
			Expect(file).To(Equal("poc--eni--hyperion--my-test-file"))
		})

		It("should upload file successfully - content type detection", func() {
			ctx := context.Background()
			ctrl := controller.NewUploadController(mockedBackend)

			var reader io.Reader = strings.NewReader("test content")
			var backendMetadata = make(map[string]string)
			backendMetadata["X-File-Checksum"] = "test-checksum"
			backendMetadata["X-File-Content-Type"] = "application/octet-stream"
			backendMetadata["X-File-Content-Type-Source"] = "auto-detected"

			mockedBackend.EXPECT().UploadFile(any(ctx), "poc--eni--hyperion--my-test-file", reader, backendMetadata).Return("poc--eni--hyperion--my-test-file", nil)

			var callMetadata = make(map[string]string)
			callMetadata["X-File-Checksum"] = "test-checksum"

			file, err := ctrl.UploadFile(ctx, "poc--eni--hyperion--my-test-file", &reader, callMetadata)
			By("returning no error and the same fileId as supplied")
			Expect(err).NotTo(HaveOccurred())
			Expect(file).To(Equal("poc--eni--hyperion--my-test-file"))
		})

		It("should upload file successfully - content type detection with filename extension", func() {
			ctx := context.Background()
			ctrl := controller.NewUploadController(mockedBackend)

			var reader io.Reader = strings.NewReader("test content")
			var backendMetadata = make(map[string]string)
			backendMetadata["X-File-Checksum"] = "test-checksum"
			backendMetadata["X-File-Content-Type"] = "text/plain; charset=utf-8"
			backendMetadata["X-File-Content-Type-Source"] = "auto-detected"

			mockedBackend.EXPECT().UploadFile(any(ctx), "poc--eni--hyperion--my-test-file.txt", reader, backendMetadata).Return("poc--eni--hyperion--my-test-file.txt", nil)

			var callMetadata = make(map[string]string)
			callMetadata["X-File-Checksum"] = "test-checksum"

			file, err := ctrl.UploadFile(ctx, "poc--eni--hyperion--my-test-file.txt", &reader, callMetadata)
			By("returning no error and the same fileId as supplied")
			Expect(err).NotTo(HaveOccurred())
			Expect(file).To(Equal("poc--eni--hyperion--my-test-file.txt"))
		})

		It("should not upload file - wrong file id", func() {
			ctx := context.Background()
			ctrl := controller.NewUploadController(mockedBackend)

			var reader io.Reader = strings.NewReader("test content")

			var callMetadata = make(map[string]string)
			callMetadata["X-File-Checksum"] = "test-checksum"

			file, err := ctrl.UploadFile(ctx, "i-dont-need-coffee", &reader, callMetadata)
			By("returning an error")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("InvalidFileId: invalid file ID 'i-dont-need-coffee'"))
			Expect(file).To(BeEmpty())
		})

		It("should not upload file - nil reader", func() {
			ctx := context.Background()
			ctrl := controller.NewUploadController(mockedBackend)

			var reader io.Reader = nil

			var callMetadata = make(map[string]string)
			callMetadata["X-File-Checksum"] = "test-checksum"

			file, err := ctrl.UploadFile(ctx, "poc--eni--hyperion--my-test-file", &reader, callMetadata)
			By("returning an error")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("UploadFailed: failed to upload file 'poc--eni--hyperion--my-test-file': file reader is nil"))
			Expect(file).To(BeEmpty())
		})

		It("should not upload file - backend error propagated", func() {
			ctx := context.Background()
			ctrl := controller.NewUploadController(mockedBackend)

			var reader io.Reader = strings.NewReader("test content")
			var backendMetadata = make(map[string]string)
			backendMetadata["X-File-Checksum"] = "test-checksum"
			backendMetadata["X-File-Content-Type"] = "application/octet-stream"
			backendMetadata["X-File-Content-Type-Source"] = "auto-detected"

			mockedBackend.EXPECT().UploadFile(any(ctx), "poc--eni--hyperion--my-test-file", reader, backendMetadata).Return("", errors.New("this is a test error message"))

			var callMetadata = make(map[string]string)
			callMetadata["X-File-Checksum"] = "test-checksum"

			file, err := ctrl.UploadFile(ctx, "poc--eni--hyperion--my-test-file", &reader, callMetadata)
			By("returning an error")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("this is a test error message"))
			Expect(file).To(BeEmpty())
		})

	})

})
