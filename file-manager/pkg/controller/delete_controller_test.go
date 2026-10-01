// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller_test

import (
	"context"

	"github.com/pkg/errors"

	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"github.com/telekom/controlplane/file-manager/pkg/controller"
	"github.com/telekom/controlplane/file-manager/test/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DeleteController", func() {
	Context("Delete controller", func() {
		mockedBackend := &mocks.MockFileDeleter{}

		It("Should delete files successfully", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			mockedBackend.EXPECT().DeleteFile(any(ctx), "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60").Return(nil)

			err := ctrl.DeleteFile(ctx, "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60")

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should return error when file not found", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			mockedBackend.EXPECT().DeleteFile(any(ctx), "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f70").
				Return(backend.ErrFileNotFound("01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f70"))

			err := ctrl.DeleteFile(ctx, "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f70")

			Expect(err).To(HaveOccurred())
			By("returning a NotFound error")
			Expect(err.Error()).To(ContainSubstring("NotFound"))
		})

		It("Should not delete files with wrong fileId format - complete nonsense", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			err := ctrl.DeleteFile(ctx, "obviously_wrong/id")
			Expect(err).To(HaveOccurred())
			By("returning the correct error message")
			Expect(err.Error()).To(BeEquivalentTo("InvalidFileId: invalid file ID 'obviously_wrong/id'"))
		})

		It("Should not delete files with wrong fileId format - truncated UUID", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			err := ctrl.DeleteFile(ctx, "01926a3e-7b2c-7d3e-8f4a")
			Expect(err).To(HaveOccurred())
			By("returning the correct error message")
			Expect(err.Error()).To(BeEquivalentTo("InvalidFileId: invalid file ID '01926a3e-7b2c-7d3e-8f4a'"))
		})

		It("Should not delete files with legacy fileId format", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			err := ctrl.DeleteFile(ctx, "poc--eni--hyperion--my-test-file.txt")
			Expect(err).To(HaveOccurred())
			By("returning the correct error message")
			Expect(err.Error()).To(BeEquivalentTo("InvalidFileId: invalid file ID 'poc--eni--hyperion--my-test-file.txt'"))
		})

		It("Should handle backend errors properly", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			mockedBackend.EXPECT().DeleteFile(any(ctx), "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f71").
				Return(errors.New("backend error"))

			err := ctrl.DeleteFile(ctx, "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f71")

			Expect(err).To(HaveOccurred())
			By("returning the backend error")
			Expect(err.Error()).To(ContainSubstring("backend error"))
		})

		It("Should reject non-canonical UUID forms", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			err := ctrl.DeleteFile(ctx, "01926A3E-7B2C-7D3E-8F4A-1B2C3D4E5F60")

			Expect(err).To(HaveOccurred())
			By("returning an InvalidFileId error")
			Expect(err.Error()).To(ContainSubstring("InvalidFileId"))
		})
	})
})
