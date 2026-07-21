// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package user

import (
	"context"
	"errors"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client/fake"
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
	userHandlerTestEnvironment           = "test"
	userHandlerTestNamespace             = "test"
	userHandlerTestInstance              = "test-instance"
	userHandlerTestName                  = "test-user"
	userHandlerTestSFTPServiceConfigName = "test-sftpserviceconfig"
)

var _ = Describe("UserHandler", func() {
	It("requires a service factory", func() {
		handler, err := New(nil)

		Expect(err).To(MatchError("service factory is required"))
		Expect(handler).To(BeNil())
	})

	It("blocks when the Instance reference is missing", func() {
		user := testUser()
		user.Spec.InstanceRef = types.ObjectRef{}
		handler, ctx, _, _ := newTestHandler()

		err := handler.CreateOrUpdate(ctx, user)

		var blocked ctrlerrors.BlockedError
		Expect(errors.As(err, &blocked)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring("Instance reference is required")))
	})

	It("keeps the User processing while the Instance is not ready", func() {
		instance := testInstance()
		user := testUser()
		handler, ctx, _, mockClient := newTestHandler()

		mockClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: userHandlerTestInstance, Namespace: userHandlerTestNamespace}, &sftpv1.Instance{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*sftpv1.Instance) = *instance
			}).
			Return(nil).
			Once()

		Expect(handler.CreateOrUpdate(ctx, user)).To(Succeed())

		processing := meta.FindStatusCondition(user.Status.Conditions, condition.ConditionTypeProcessing)
		Expect(processing).To(BeNil())
		ready := meta.FindStatusCondition(user.Status.Conditions, condition.ConditionTypeReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("WaitingForInstance"))
		Expect(meta.IsStatusConditionFalse(user.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
	})

	It("syncs only current User SSH public keys", func() {
		instance := testInstanceWithReadyStatus()
		user := testUser()
		user.Spec.SSHPublicKeys = []string{"ssh-rsa cHJvdmlkZXI= provider@example.com"}
		handler, ctx, mockService, mockClient := newTestHandler()

		mockClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: userHandlerTestInstance, Namespace: userHandlerTestNamespace}, &sftpv1.Instance{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*sftpv1.Instance) = *instance
			}).
			Return(nil).
			Once()

		var capturedClientID string
		var capturedKeys service.ClientPublicKeyMap
		mockService.EXPECT().UpdatePublicKeysForSFTPUser(mock.Anything, instance.Name, mock.Anything, mock.Anything).
			Run(func(_ context.Context, _, clientID string, keys service.ClientPublicKeyMap) {
				capturedClientID = clientID
				capturedKeys = keys
			}).
			Return(nil).
			Once()

		Expect(handler.CreateOrUpdate(ctx, user)).To(Succeed())

		Expect(capturedClientID).To(Equal(user.Namespace + "/" + user.Name))
		Expect(capturedKeys).To(HaveLen(1))
		Expect(capturedKeys).To(HaveKey("items"))
		Expect(capturedKeys["items"]).To(ConsistOf(service.RoverPublicKeyModel{
			PublicKey:    "ssh-rsa cHJvdmlkZXI= provider@example.com",
			SftpUserName: instance.Name,
			Description:  ptrTo(user.Namespace + "/" + user.Name + "/0"),
		}))
		Expect(meta.IsStatusConditionFalse(user.Status.Conditions, condition.ConditionTypeProcessing)).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(user.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		ready := meta.FindStatusCondition(user.Status.Conditions, condition.ConditionTypeReady)
		Expect(ready.Reason).To(Equal("SSHPublicKeysUpdated"))
	})

	It("accepts SSH keys as provided and marks the User ready", func() {
		instance := testInstanceWithReadyStatus()
		user := testUser()
		user.Spec.SSHPublicKeys = []string{
			"invalid",
			"ssh-rsa cHJvdmlkZXI= provider@example.com",
		}
		handler, ctx, mockService, mockClient := newTestHandler()

		mockClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: userHandlerTestInstance, Namespace: userHandlerTestNamespace}, &sftpv1.Instance{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*sftpv1.Instance) = *instance
			}).
			Return(nil).
			Once()

		var capturedKeys service.ClientPublicKeyMap
		mockService.EXPECT().UpdatePublicKeysForSFTPUser(mock.Anything, instance.Name, user.Namespace+"/"+user.Name, mock.Anything).
			Run(func(_ context.Context, _, _ string, keys service.ClientPublicKeyMap) {
				capturedKeys = keys
			}).
			Return(nil).
			Once()

		Expect(handler.CreateOrUpdate(ctx, user)).To(Succeed())

		processing := meta.FindStatusCondition(user.Status.Conditions, condition.ConditionTypeProcessing)
		Expect(processing).NotTo(BeNil())
		Expect(processing.Status).To(Equal(metav1.ConditionFalse))
		Expect(processing.Reason).To(Equal("Done"))
		Expect(meta.IsStatusConditionTrue(user.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		Expect(capturedKeys).To(HaveKey("items"))
		Expect(capturedKeys["items"]).To(HaveLen(2))
	})

	It("returns synchronization errors", func() {
		instance := testInstanceWithReadyStatus()
		user := testUser()
		user.Spec.SSHPublicKeys = []string{"ssh-rsa cHJvdmlkZXI= provider@example.com"}
		handler, ctx, mockService, mockClient := newTestHandler()

		mockClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: userHandlerTestInstance, Namespace: userHandlerTestNamespace}, &sftpv1.Instance{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*sftpv1.Instance) = *instance
			}).
			Return(nil).
			Once()

		mockService.EXPECT().UpdatePublicKeysForSFTPUser(mock.Anything, instance.Name, user.Namespace+"/"+user.Name, mock.Anything).
			Return(errors.New("dds unavailable")).
			Once()

		err := handler.CreateOrUpdate(ctx, user)

		Expect(err).To(MatchError(ContainSubstring("updating public keys for User")))
		Expect(err).To(MatchError(ContainSubstring("dds unavailable")))
	})

	It("removes User keys from service on delete", func() {
		instance := testInstanceWithReadyStatus()
		user := testUser()
		handler, ctx, mockService, mockClient := newTestHandler()

		mockClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: userHandlerTestInstance, Namespace: userHandlerTestNamespace}, &sftpv1.Instance{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*sftpv1.Instance) = *instance
			}).
			Return(nil).
			Once()

		mockService.EXPECT().UpdatePublicKeysForSFTPUser(mock.Anything, instance.Name, user.Namespace+"/"+user.Name, service.ClientPublicKeyMap{
			"items": []service.RoverPublicKeyModel{},
		}).Return(nil).Once()

		Expect(handler.Delete(ctx, user)).To(Succeed())
	})

	It("does not fail delete when referenced Instance does not exist", func() {
		user := testUser()
		handler, ctx, _, mockClient := newTestHandler()

		mockClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: userHandlerTestInstance, Namespace: userHandlerTestNamespace}, &sftpv1.Instance{}).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: sftpv1.GroupVersion.Group, Resource: "instances"}, userHandlerTestInstance)).
			Once()

		Expect(handler.Delete(ctx, user)).To(Succeed())
	})
})

func newTestHandler() (*UserHandler, context.Context, *sftpmocks.MockService, *fake.MockJanitorClient) {
	mockClient := fake.NewMockJanitorClient(GinkgoT())
	ctx := cclient.WithClient(context.Background(), mockClient)

	mockService := sftpmocks.NewMockService(GinkgoT())
	handler, err := New(recordingFactory{svc: mockService})
	Expect(err).NotTo(HaveOccurred())

	return handler, ctx, mockService, mockClient
}

func testInstance() *sftpv1.Instance {
	return &sftpv1.Instance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: sftpv1.GroupVersion.String(),
			Kind:       "Instance",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       userHandlerTestInstance,
			Namespace:  userHandlerTestNamespace,
			Generation: 1,
		},
		Spec: sftpv1.InstanceSpec{
			SFTPServiceConfigRef: types.ObjectRef{
				Name:      userHandlerTestSFTPServiceConfigName,
				Namespace: userHandlerTestNamespace,
			},
		},
	}
}

func testInstanceWithReadyStatus() *sftpv1.Instance {
	instance := testInstance()
	ready := condition.NewReadyCondition("InstanceProvided", "Instance has been provided")
	ready.ObservedGeneration = instance.Generation
	instance.SetCondition(ready)
	return instance
}

func testUser() *sftpv1.User {
	return testUserNamed(userHandlerTestName)
}

func testUserNamed(name string) *sftpv1.User {
	return &sftpv1.User{
		TypeMeta: metav1.TypeMeta{
			APIVersion: sftpv1.GroupVersion.String(),
			Kind:       "User",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  userHandlerTestNamespace,
			Generation: 1,
		},
		Spec: sftpv1.UserSpec{
			InstanceRef: types.ObjectRef{
				Name:      userHandlerTestInstance,
				Namespace: userHandlerTestNamespace,
			},
		},
	}
}

func ptrTo[T any](value T) *T {
	return &value
}

type recordingFactory struct {
	svc service.Service
	err error
}

func (f recordingFactory) ServiceFor(context.Context, client.ObjectKey) (service.Service, error) {
	return f.svc, f.err
}
