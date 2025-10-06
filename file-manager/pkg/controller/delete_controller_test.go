// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"github.com/telekom/controlplane/file-manager/pkg/controller"
	"github.com/telekom/controlplane/file-manager/test/mocks"
)

var _ = Describe("DeleteController", func() {

	Context("Delete controller", func() {

		var mockedBackend = &mocks.MockFileDeleter{}

		It("Should delete files successfully", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			mockedBackend.EXPECT().DeleteFile(any(ctx), "poc/eni/hyperion/my-test-file.txt").Return(nil)

			err := ctrl.DeleteFile(ctx, "poc--eni--hyperion--my-test-file.txt")

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should return error when file not found", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			mockedBackend.EXPECT().DeleteFile(any(ctx), "poc/eni/hyperion/nonexistent.txt").
				Return(backend.ErrFileNotFound("poc/eni/hyperion/nonexistent.txt"))

			err := ctrl.DeleteFile(ctx, "poc--eni--hyperion--nonexistent.txt")

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

		It("Should not delete files with wrong fileId format - wrong number of parts", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			err := ctrl.DeleteFile(ctx, "poc--eni--fileId")
			Expect(err).To(HaveOccurred())
			By("returning the correct error message")
			Expect(err.Error()).To(BeEquivalentTo("InvalidFileId: invalid file ID 'poc--eni--fileId'"))
		})

		It("Should handle files with slashes in filename (nested paths)", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			// Slashes in the filename part are allowed and converted to path separators
			mockedBackend.EXPECT().DeleteFile(any(ctx), "poc/eni/team/file/with/slashes.txt").
				Return(nil)

			err := ctrl.DeleteFile(ctx, "poc--eni--team--file/with/slashes.txt")
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should handle backend errors properly", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			mockedBackend.EXPECT().DeleteFile(any(ctx), "poc/eni/hyperion/error-file.txt").
				Return(errors.New("backend error"))

			err := ctrl.DeleteFile(ctx, "poc--eni--hyperion--error-file.txt")

			Expect(err).To(HaveOccurred())
			By("returning the backend error")
			Expect(err.Error()).To(ContainSubstring("backend error"))
		})

		It("Should return error when fileId has empty parts (conversion fails)", func() {
			ctx := context.Background()
			ctrl := controller.NewDeleteController(mockedBackend)

			// FileId with empty parts - has correct number of separators but empty values
			err := ctrl.DeleteFile(ctx, "poc----file.txt")

			Expect(err).To(HaveOccurred())
			By("returning an InvalidFileId error")
			Expect(err.Error()).To(ContainSubstring("InvalidFileId"))
			Expect(err.Error()).To(ContainSubstring("poc----file.txt"))
		})

	})

})
