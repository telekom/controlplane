// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	cstore "github.com/telekom/controlplane/common-server/pkg/store"
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

var _ = Describe("AgentSpecification Controller", func() {
	const (
		resourceId = "eni--hyperion--eni-agent-v1"
		namespace  = "poc--eni--hyperion"
		name       = "eni-agent-v1"
		fileId     = "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f64"
	)

	var (
		agentCtx   context.Context
		agentStore *mocks.MockObjectStore[*roverv1.AgentSpecification]
		agentFiles *filefake.MockFileManager
		ctrl       *AgentSpecificationControllerImpl
		specMap    map[string]any
		specYAML   []byte
	)

	isUUIDv7 := mock.MatchedBy(func(id string) bool {
		parsed, err := uuid.Parse(id)
		return err == nil && parsed.Version() == 7
	})

	existing := func(hash string) *roverv1.AgentSpecification {
		return &roverv1.AgentSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: roverv1.AgentSpecificationSpec{
				BasePath:      "/eni/agent/v1",
				Specification: fileId,
				Hash:          hash,
			},
		}
	}

	expectDownload := func(id string) {
		agentFiles.EXPECT().DownloadFile(mock.Anything, id, mock.Anything).
			RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
				_, err := w.Write(specYAML)
				return &fileApi.FileDownloadResponse{}, err
			}).Once()
	}

	BeforeEach(func() {
		agentCtx = security.ToContext(context.Background(), &security.BusinessContext{
			Environment: "poc", Group: "eni", Team: "hyperion",
		})
		agentStore = mocks.NewMockObjectStore[*roverv1.AgentSpecification](GinkgoT())
		agentFiles = filefake.NewMockFileManager(GinkgoT())
		file.GetFileManager = func() fileApi.FileManager { return agentFiles }
		ctrl = NewAgentSpecificationController(&store.Stores{AgentSpecificationStore: agentStore})

		specMap = map[string]any{"basePath": "/eni/agent/v1", "info": map[string]any{"version": "2.0.0"}}
		var err error
		specYAML, err = yaml.Marshal(specMap)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		file.GetFileManager = func() fileApi.FileManager { return mockFileManager }
		cconfig.SetFeatureEnabled(cconfig.FeatureFileManager, true)
	})

	It("should not implement Create", func() {
		_, err := ctrl.Create(agentCtx, api.AgentSpecificationCreateRequest{})
		Expect(err).To(HaveOccurred())
	})

	Context("Delete", func() {
		It("should delete the stored file and the resource", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(""), nil).Once()
			agentFiles.EXPECT().DeleteFile(mock.Anything, fileId).Return(nil).Once()
			agentStore.EXPECT().Delete(mock.Anything, namespace, name).Return(nil).Once()

			Expect(ctrl.Delete(agentCtx, resourceId)).To(Succeed())
		})

		It("should tolerate a missing file", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(""), nil).Once()
			agentFiles.EXPECT().DeleteFile(mock.Anything, fileId).Return(file.ErrNotFound).Once()
			agentStore.EXPECT().Delete(mock.Anything, namespace, name).Return(nil).Once()

			Expect(ctrl.Delete(agentCtx, resourceId)).To(Succeed())
		})

		It("should return file-manager errors", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(""), nil).Once()
			agentFiles.EXPECT().DeleteFile(mock.Anything, fileId).Return(errors.New("boom")).Once()

			Expect(ctrl.Delete(agentCtx, resourceId)).To(MatchError("boom"))
		})

		It("should return NotFound without touching file-manager when the resource is missing", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(nil, problems.NotFound(name)).Once()
			agentStore.EXPECT().Delete(mock.Anything, namespace, name).Return(problems.NotFound(name)).Once()

			err := ctrl.Delete(agentCtx, resourceId)
			Expect(problems.IsNotFound(err)).To(BeTrue())
		})

		It("should return store errors when looking up the resource", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(nil, errors.New("store down")).Once()

			Expect(ctrl.Delete(agentCtx, resourceId)).To(MatchError("store down"))
		})

		It("should skip file-manager when the feature is disabled", func() {
			cconfig.SetFeatureEnabled(cconfig.FeatureFileManager, false)
			agentStore.EXPECT().Delete(mock.Anything, namespace, name).Return(errors.New("store down")).Once()

			Expect(ctrl.Delete(agentCtx, resourceId)).To(MatchError("store down"))
		})

		It("should reject an invalid resource id", func() {
			Expect(ctrl.Delete(agentCtx, "invalid")).NotTo(Succeed())
		})
	})

	Context("Get", func() {
		It("should return the specification content", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(""), nil).Once()
			expectDownload(fileId)

			res, err := ctrl.Get(agentCtx, resourceId)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Id).To(Equal(resourceId))
			Expect(res.Specification).To(HaveKeyWithValue("basePath", "/eni/agent/v1"))
		})

		It("should return NotFound", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(nil, problems.NotFound(name)).Once()

			_, err := ctrl.Get(agentCtx, resourceId)
			Expect(problems.IsNotFound(err)).To(BeTrue())
		})

		It("should return download errors", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(""), nil).Once()
			agentFiles.EXPECT().DownloadFile(mock.Anything, fileId, mock.Anything).Return(nil, errors.New("boom")).Once()

			_, err := ctrl.Get(agentCtx, resourceId)
			var problem problems.Problem
			Expect(errors.As(err, &problem)).To(BeTrue())
			Expect(problem.Code()).To(Equal(http.StatusInternalServerError))
			Expect(err).To(MatchError("Failed to download agent specification: boom"))
		})

		It("should skip the download when the feature is disabled", func() {
			cconfig.SetFeatureEnabled(cconfig.FeatureFileManager, false)
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(""), nil).Once()

			res, err := ctrl.Get(agentCtx, resourceId)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Specification).To(BeEmpty())
		})
	})

	Context("GetAll", func() {
		It("should return all specifications", func() {
			agentStore.EXPECT().List(mock.Anything, mock.Anything).Return(
				&cstore.ListResponse[*roverv1.AgentSpecification]{Items: []*roverv1.AgentSpecification{existing("")}}, nil).Once()
			expectDownload(fileId)

			res, err := ctrl.GetAll(agentCtx, api.GetAllAgentSpecificationsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Items).To(HaveLen(1))
		})

		It("should return store errors", func() {
			agentStore.EXPECT().List(mock.Anything, mock.Anything).Return(nil, errors.New("store down")).Once()

			_, err := ctrl.GetAll(agentCtx, api.GetAllAgentSpecificationsParams{})
			Expect(err).To(MatchError("store down"))
		})

		It("should return download errors as problems", func() {
			agentStore.EXPECT().List(mock.Anything, mock.Anything).Return(
				&cstore.ListResponse[*roverv1.AgentSpecification]{Items: []*roverv1.AgentSpecification{existing("")}}, nil).Once()
			agentFiles.EXPECT().DownloadFile(mock.Anything, fileId, mock.Anything).Return(nil, errors.New("boom")).Once()

			_, err := ctrl.GetAll(agentCtx, api.GetAllAgentSpecificationsParams{})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Update", func() {
		It("should upload new specifications under a new UUIDv7", func() {
			var stored *roverv1.AgentSpecification
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(nil, problems.NotFound(name)).Once()
			agentFiles.EXPECT().UploadFile(mock.Anything, isUUIDv7, "application/yaml", mock.Anything).
				RunAndReturn(func(_ context.Context, id, _ string, _ io.Reader) (*fileApi.FileUploadResponse, error) {
					return &fileApi.FileUploadResponse{FileId: id, FileHash: "hash"}, nil
				}).Once()
			agentStore.EXPECT().CreateOrReplace(mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, obj *roverv1.AgentSpecification) error {
					stored = obj
					return nil
				}).Once()
			agentStore.EXPECT().Get(mock.Anything, namespace, name).
				RunAndReturn(func(context.Context, string, string) (*roverv1.AgentSpecification, error) { return stored, nil }).Once()
			agentFiles.EXPECT().DownloadFile(mock.Anything, isUUIDv7, mock.Anything).
				RunAndReturn(func(_ context.Context, _ string, w io.Writer) (*fileApi.FileDownloadResponse, error) {
					_, err := w.Write(specYAML)
					return &fileApi.FileDownloadResponse{}, err
				}).Once()

			_, err := ctrl.Update(agentCtx, resourceId, api.AgentSpecificationUpdateRequest{Specification: specMap})
			Expect(err).NotTo(HaveOccurred())
			Expect(stored.Namespace).To(Equal(namespace))
			Expect(stored.Spec.Version).To(Equal("2.0.0"))
		})

		It("should reuse the stored file id and skip the upload when content is unchanged", func() {
			unchanged := existing(computeHash(specYAML))
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(unchanged, nil).Times(2)
			agentStore.EXPECT().CreateOrReplace(mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, obj *roverv1.AgentSpecification) error {
					Expect(obj.Spec.Specification).To(Equal(fileId))
					return nil
				}).Once()
			expectDownload(fileId)

			_, err := ctrl.Update(agentCtx, resourceId, api.AgentSpecificationUpdateRequest{Specification: specMap})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reuse the stored file id when content changed", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing("outdated"), nil).Times(2)
			agentFiles.EXPECT().UploadFile(mock.Anything, fileId, "application/yaml", mock.Anything).
				Return(&fileApi.FileUploadResponse{FileId: fileId, FileHash: "hash"}, nil).Once()
			agentStore.EXPECT().CreateOrReplace(mock.Anything, mock.Anything).Return(nil).Once()
			expectDownload(fileId)

			_, err := ctrl.Update(agentCtx, resourceId, api.AgentSpecificationUpdateRequest{Specification: specMap})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return upload errors", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(nil, problems.NotFound(name)).Once()
			agentFiles.EXPECT().UploadFile(mock.Anything, isUUIDv7, "application/yaml", mock.Anything).
				Return(nil, errors.New("boom")).Once()

			_, err := ctrl.Update(agentCtx, resourceId, api.AgentSpecificationUpdateRequest{Specification: specMap})
			Expect(err).To(MatchError("boom"))
		})

		It("should return store errors from the hash lookup", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(nil, errors.New("store down")).Once()

			_, err := ctrl.Update(agentCtx, resourceId, api.AgentSpecificationUpdateRequest{Specification: specMap})
			Expect(err).To(MatchError("store down"))
		})

		It("should return store errors on persist", func() {
			cconfig.SetFeatureEnabled(cconfig.FeatureFileManager, false)
			agentStore.EXPECT().CreateOrReplace(mock.Anything, mock.Anything).Return(errors.New("store down")).Once()

			_, err := ctrl.Update(agentCtx, resourceId, api.AgentSpecificationUpdateRequest{Specification: specMap})
			Expect(err).To(MatchError("store down"))
		})

		It("should reject a specification without basePath", func() {
			_, err := ctrl.Update(agentCtx, resourceId, api.AgentSpecificationUpdateRequest{Specification: map[string]any{"info": map[string]any{}}})
			Expect(err).To(HaveOccurred())
		})

		It("should reject a name that does not match the resource id", func() {
			_, err := ctrl.Update(agentCtx, resourceId, api.AgentSpecificationUpdateRequest{Specification: map[string]any{"basePath": "/other/v1"}})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("GetStatus", func() {
		It("should return the status", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(existing(""), nil).Once()

			_, err := ctrl.GetStatus(agentCtx, resourceId)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return NotFound", func() {
			agentStore.EXPECT().Get(mock.Anything, namespace, name).Return(nil, problems.NotFound(name)).Once()

			_, err := ctrl.GetStatus(agentCtx, resourceId)
			Expect(problems.IsNotFound(err)).To(BeTrue())
		})
	})
})
