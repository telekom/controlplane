// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	fileApi "github.com/telekom/controlplane/file-manager/api"
	filefake "github.com/telekom/controlplane/file-manager/api/fake"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiChangelog and Roadmap Controller file-manager handling", func() {
	const (
		resourceId = "eni--hyperion--eni-test-api"
		namespace  = "poc--eni--hyperion"
		name       = "eni-test-api"
		basePath   = "/eni/test-api/v1"
		fileId     = "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f66"
	)

	var (
		fmCtx context.Context
		files *filefake.MockFileManager
	)

	BeforeEach(func() {
		fmCtx = security.ToContext(context.Background(), &security.BusinessContext{
			Environment: "poc", Group: "eni", Team: "hyperion",
		})
		files = filefake.NewMockFileManager(GinkgoT())
		file.GetFileManager = func() fileApi.FileManager { return files }
	})

	AfterEach(func() {
		file.GetFileManager = func() fileApi.FileManager { return mockFileManager }
		cconfig.SetFeatureEnabled(cconfig.FeatureFileManager, true)
	})

	Context("ApiChangelog", func() {
		var (
			changelogStore *mocks.MockObjectStore[*roverv1.ApiChangelog]
			ctrl           *ApiChangelogController
		)

		existing := func(contents string) *roverv1.ApiChangelog {
			return &roverv1.ApiChangelog{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec:       roverv1.ApiChangelogSpec{Contents: contents},
			}
		}

		BeforeEach(func() {
			changelogStore = mocks.NewMockObjectStore[*roverv1.ApiChangelog](GinkgoT())
			ctrl = NewApiChangelogController(&store.Stores{ApiChangelogStore: changelogStore})
		})

		Context("file-manager disabled", func() {
			BeforeEach(func() {
				cconfig.SetFeatureEnabled(cconfig.FeatureFileManager, false)
			})

			It("should reject the ApiChangelog without calling the store or the file-manager", func() {
				items := []api.ApiChangelogItem{{Date: "2024-03-15", Version: "1.2.3", Description: "Fixes"}}
				_, err := ctrl.Update(fmCtx, resourceId, api.ApiChangelogUpdateRequest{BasePath: basePath, Items: items})

				var problem problems.Problem
				Expect(errors.As(err, &problem)).To(BeTrue())
				Expect(problem.Code()).To(Equal(http.StatusBadRequest))
				Expect(err).To(MatchError("Bad Request: ApiChangelog requires the file manager, which is disabled in this environment"))
			})

			It("should delete the ApiChangelog without calling the file-manager", func() {
				changelogStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(fileId), nil).Once()
				changelogStore.EXPECT().Delete(mock.Anything, namespace, name).Return(nil).Once()

				Expect(ctrl.Delete(fmCtx, resourceId)).To(Succeed())
			})
		})

		Context("file-manager enabled", func() {
			It("should delete the ApiChangelog when the stored file is already missing", func() {
				changelogStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(fileId), nil).Once()
				files.EXPECT().DeleteFile(mock.Anything, fileId).Return(file.ErrNotFound).Once()
				changelogStore.EXPECT().Delete(mock.Anything, namespace, name).Return(nil).Once()

				Expect(ctrl.Delete(fmCtx, resourceId)).To(Succeed())
			})

			It("should skip the download for an ApiChangelog created while the file-manager was disabled", func() {
				changelogStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(""), nil).Once()

				res, err := ctrl.Get(fmCtx, resourceId)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.Items).To(BeEmpty())
			})

			It("should skip the file-manager when deleting an ApiChangelog without a stored file", func() {
				changelogStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(""), nil).Once()
				changelogStore.EXPECT().Delete(mock.Anything, namespace, name).Return(nil).Once()

				Expect(ctrl.Delete(fmCtx, resourceId)).To(Succeed())
			})

			It("should return NotFound for a missing ApiChangelog", func() {
				changelogStore.EXPECT().Get(mock.Anything, namespace, name).Return(nil, problems.NotFound(name)).Once()

				err := ctrl.Delete(fmCtx, resourceId)
				Expect(problems.IsNotFound(err)).To(BeTrue())
			})
		})
	})

	Context("Roadmap", func() {
		var (
			roadmapStore *mocks.MockObjectStore[*roverv1.Roadmap]
			ctrl         *RoadmapController
		)

		existing := func(contents string) *roverv1.Roadmap {
			return &roverv1.Roadmap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec:       roverv1.RoadmapSpec{Contents: contents},
			}
		}

		BeforeEach(func() {
			roadmapStore = mocks.NewMockObjectStore[*roverv1.Roadmap](GinkgoT())
			ctrl = NewRoadmapController(&store.Stores{RoadmapStore: roadmapStore})
		})

		Context("file-manager disabled", func() {
			BeforeEach(func() {
				cconfig.SetFeatureEnabled(cconfig.FeatureFileManager, false)
			})

			It("should reject the Roadmap without calling the store or the file-manager", func() {
				items := []api.ApiRoadmapItem{{Date: "Q1 2024", Title: "MVP Release", Description: "Initial release"}}
				_, err := ctrl.Update(fmCtx, resourceId, api.ApiRoadmapUpdateRequest{BasePath: basePath, Items: items})

				var problem problems.Problem
				Expect(errors.As(err, &problem)).To(BeTrue())
				Expect(problem.Code()).To(Equal(http.StatusBadRequest))
				Expect(err).To(MatchError("Bad Request: ApiRoadmap requires the file manager, which is disabled in this environment"))
			})

			It("should delete the Roadmap without calling the file-manager", func() {
				roadmapStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(fileId), nil).Once()
				roadmapStore.EXPECT().Delete(mock.Anything, namespace, name).Return(nil).Once()

				Expect(ctrl.Delete(fmCtx, resourceId)).To(Succeed())
			})

			It("should skip the download when the file-manager is disabled", func() {
				roadmapStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(fileId), nil).Once()

				res, err := ctrl.Get(fmCtx, resourceId)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.Items).To(BeEmpty())
			})
		})

		Context("file-manager enabled", func() {
			It("should delete the Roadmap when the stored file is already missing", func() {
				roadmapStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(fileId), nil).Once()
				files.EXPECT().DeleteFile(mock.Anything, fileId).Return(file.ErrNotFound).Once()
				roadmapStore.EXPECT().Delete(mock.Anything, namespace, name).Return(nil).Once()

				Expect(ctrl.Delete(fmCtx, resourceId)).To(Succeed())
			})

			It("should skip the download for a Roadmap created while the file-manager was disabled", func() {
				roadmapStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(""), nil).Once()

				res, err := ctrl.Get(fmCtx, resourceId)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.Items).To(BeEmpty())
			})

			It("should skip the file-manager when deleting a Roadmap without a stored file", func() {
				roadmapStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(""), nil).Once()
				roadmapStore.EXPECT().Delete(mock.Anything, namespace, name).Return(nil).Once()

				Expect(ctrl.Delete(fmCtx, resourceId)).To(Succeed())
			})
		})
	})
})
