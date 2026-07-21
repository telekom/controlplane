// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"context"
	"errors"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	"github.com/telekom/controlplane/sftp/internal/service"
	sftpmocks "github.com/telekom/controlplane/sftp/test/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	instanceHandlerTestNamespace             = "test"
	instanceHandlerTestName                  = "test-instance"
	instanceHandlerTestSFTPServiceConfigName = "test-sftpServiceConfig"
)

var _ = Describe("InstanceHandler", func() {
	It("requires a service factory", func() {
		handler, err := New(nil)

		Expect(err).To(MatchError("service factory is required"))
		Expect(handler).To(BeNil())
	})

	It("marks an Instance ready when its SFTPServiceConfig exists", func() {
		handler, ctx, instance, mockService := newTestHandler()
		expectCreateOrUpdateSFTPUser(mockService, nil, nil)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())
		Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, condition.ConditionTypeProcessing)).To(BeTrue())
	})

	It("blocks when SFTPServiceConfig reference is missing", func() {
		handler, ctx, instance, _ := newTestHandler()
		instance.Spec.SFTPServiceConfigRef = types.ObjectRef{}

		err := handler.CreateOrUpdate(ctx, instance)

		var blocked ctrlerrors.BlockedError
		Expect(errors.As(err, &blocked)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring("SFTPServiceConfig reference is required")))
	})

	It("retries when the referenced SFTPServiceConfig service is unavailable", func() {
		handler, ctx, instance, _ := newTestHandlerWithFactory(recordingFactory{
			err: ctrlerrors.RetryableErrorf("SFTP client for SFTPServiceConfig %q is not initialized", "test/test-sftpServiceConfig"),
		})

		err := handler.CreateOrUpdate(ctx, instance)

		var retryable ctrlerrors.RetryableError
		Expect(errors.As(err, &retryable)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring("SFTPServiceConfig")))
		Expect(err).To(MatchError(ContainSubstring("not initialized")))
	})

	It("creates a service user with description and empty Horizon notification events", func() {
		handler, ctx, instance, mockService := newTestHandler()
		instance.Spec.Description = "Team transfer user"
		createdModel := service.RoverSftpUserModel{}
		expectCreateOrUpdateSFTPUser(mockService, &createdModel, nil)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())

		Expect(createdModel.SftpUserName).To(Equal(instanceHandlerTestName))
		Expect(createdModel.Description).NotTo(BeNil())
		Expect(*createdModel.Description).To(Equal("Team transfer user"))
		Expect(createdModel.HorizonNotificationEvents).NotTo(BeNil())
		Expect(*createdModel.HorizonNotificationEvents).To(BeEmpty())
	})

	It("skips service user provisioning when the Ready condition observed generation is current", func() {
		handler, ctx, instance, _ := newTestHandler()
		ready := condition.NewReadyCondition("InstanceProvided", "Instance has been provided")
		ready.ObservedGeneration = instance.Generation
		instance.SetCondition(ready)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())
	})

	It("provisions the service user when the Ready condition observed generation is stale", func() {
		handler, ctx, instance, mockService := newTestHandler()
		ready := condition.NewReadyCondition("InstanceProvided", "Instance has been provided")
		ready.ObservedGeneration = instance.Generation - 1
		instance.SetCondition(ready)
		expectCreateOrUpdateSFTPUser(mockService, nil, nil)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())
	})

	It("returns service user creation errors", func() {
		handler, ctx, instance, mockService := newTestHandler()
		expectCreateOrUpdateSFTPUser(mockService, nil, errors.New("create failed"))

		err := handler.CreateOrUpdate(ctx, instance)

		Expect(err).To(MatchError(ContainSubstring("creating or updating SFTP user")))
		Expect(err).To(MatchError(ContainSubstring("create failed")))
	})

	It("deletes the instance SFTP user", func() {
		handler, ctx, instance, mockService := newTestHandler()
		expectDeleteSFTPUser(mockService, nil)

		Expect(handler.Delete(ctx, instance)).To(Succeed())
	})

	It("does not delete a service user when SFTPServiceConfig reference is missing", func() {
		handler, ctx, instance, _ := newTestHandler()
		instance.Spec.SFTPServiceConfigRef = types.ObjectRef{}

		Expect(handler.Delete(ctx, instance)).To(Succeed())
	})

	It("returns service lookup errors during deletion", func() {
		handler, ctx, instance, _ := newTestHandlerWithFactory(recordingFactory{err: errors.New("service unavailable")})

		err := handler.Delete(ctx, instance)

		Expect(err).To(MatchError("service unavailable"))
	})

	It("wraps service user deletion errors", func() {
		handler, ctx, instance, mockService := newTestHandler()
		expectDeleteSFTPUser(mockService, errors.New("delete failed"))

		err := handler.Delete(ctx, instance)

		Expect(err).To(MatchError(ContainSubstring("deleting SFTP user")))
		Expect(err).To(MatchError(ContainSubstring("delete failed")))
	})

})

func newTestHandler(objects ...client.Object) (*InstanceHandler, context.Context, *sftpv1.Instance, *sftpmocks.MockService) {
	return newTestHandlerWithFactory(nil, objects...)
}

func newTestHandlerWithFactory(factory service.Factory, objects ...client.Object) (*InstanceHandler, context.Context, *sftpv1.Instance, *sftpmocks.MockService) {
	instance := testInstance()
	mockService := sftpmocks.NewMockService(GinkgoT())

	mockClient := fakeclient.NewMockJanitorClient(GinkgoT())

	if factory == nil {
		factory = recordingFactory{svc: mockService}
	}
	handler, err := New(factory)
	Expect(err).NotTo(HaveOccurred())
	ctx := cclient.WithClient(context.Background(), mockClient)
	return handler, ctx, instance, mockService
}

func testInstance() *sftpv1.Instance {
	return &sftpv1.Instance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: sftpv1.GroupVersion.String(),
			Kind:       "Instance",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       instanceHandlerTestName,
			Namespace:  instanceHandlerTestNamespace,
			Generation: 1,
		},
		Spec: sftpv1.InstanceSpec{
			SFTPServiceConfigRef: types.ObjectRef{
				Name:      instanceHandlerTestSFTPServiceConfigName,
				Namespace: instanceHandlerTestNamespace,
			},
		},
	}
}

func expectCreateOrUpdateSFTPUser(mockService *sftpmocks.MockService, createdModel *service.RoverSftpUserModel, err error) {
	call := mockService.EXPECT().CreateOrUpdateSFTPUser(mock.Anything, mock.Anything)
	if createdModel != nil {
		call.Run(func(_ context.Context, user service.RoverSftpUserModel) {
			*createdModel = user
		})
	}
	call.Return(err).Once()
}

func expectDeleteSFTPUser(mockService *sftpmocks.MockService, err error) {
	mockService.EXPECT().DeleteSFTPUser(mock.Anything, instanceHandlerTestName).Return(err).Once()
}

type recordingFactory struct {
	svc service.Service
	err error
}

func (f recordingFactory) ServiceFor(context.Context, client.ObjectKey) (service.Service, error) {
	return f.svc, f.err
}
