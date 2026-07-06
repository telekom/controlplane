// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/mock"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	"github.com/telekom/controlplane/sftp/internal/service"
	sftpmocks "github.com/telekom/controlplane/sftp/test/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	instanceHandlerTestEnvironment           = "test"
	instanceHandlerTestNamespace             = "test"
	instanceHandlerTestName                  = "test-instance"
	instanceHandlerTestSFTPServiceConfigName = "test-sftpServiceConfig"
	instanceHandlerTestUserName              = "test-user"
)

var _ = Describe("InstanceHandler", func() {
	It("marks an Instance ready when its SFTPServiceConfig exists", func() {
		handler, ctx, instance, mockService := newTestHandler()
		expectCreateOrUpdateSFTPUser(mockService, nil, nil)
		expectUpdatePublicKeysForSFTPUser(mockService, nil, nil)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())
		Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, condition.ConditionTypeProcessing)).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, sftpv1.ConditionTypePublicKeysUpdatedInService)).To(BeTrue())
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
		handler, ctx, instance, _ := newTestHandler()
		handler.ServiceFactory = recordingFactory{
			err: ctrlerrors.RetryableErrorf("SFTP client for SFTPServiceConfig %q is not initialized", "test/test-sftpServiceConfig"),
		}

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
		expectUpdatePublicKeysForSFTPUser(mockService, nil, nil)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())

		Expect(createdModel.SftpUserName).To(Equal(instanceHandlerTestName))
		Expect(createdModel.Description).NotTo(BeNil())
		Expect(*createdModel.Description).To(Equal("Team transfer user"))
		Expect(createdModel.HorizonNotificationEvents).NotTo(BeNil())
		Expect(*createdModel.HorizonNotificationEvents).To(BeEmpty())
	})

	It("skips service user provisioning when the Ready condition observed generation is current", func() {
		handler, ctx, instance, mockService := newTestHandler()
		ready := condition.NewReadyCondition("InstanceProvided", "Instance has been provided")
		ready.ObservedGeneration = instance.Generation
		instance.SetCondition(ready)
		expectUpdatePublicKeysForSFTPUser(mockService, nil, nil)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())
	})

	It("provisions the service user when the Ready condition observed generation is stale", func() {
		handler, ctx, instance, mockService := newTestHandler()
		ready := condition.NewReadyCondition("InstanceProvided", "Instance has been provided")
		ready.ObservedGeneration = instance.Generation - 1
		instance.SetCondition(ready)
		expectCreateOrUpdateSFTPUser(mockService, nil, nil)
		expectUpdatePublicKeysForSFTPUser(mockService, nil, nil)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())
	})

	It("returns service user creation errors", func() {
		handler, ctx, instance, mockService := newTestHandler()
		expectCreateOrUpdateSFTPUser(mockService, nil, errors.New("create failed"))

		err := handler.CreateOrUpdate(ctx, instance)

		Expect(err).To(MatchError(ContainSubstring("creating or updating SFTP user")))
		Expect(err).To(MatchError(ContainSubstring("create failed")))
	})

	It("syncs user SSH public keys to DDS grouped by instance name", func() {
		publicKey := "ssh-rsa cHJvdmlkZXI= provider@example.com"

		user := testUser()
		user.Spec.SSHPublicKeys = []string{publicKey}

		handler, ctx, instance, mockService := newTestHandler(user)
		var keys service.ClientPublicKeyMap
		expectCreateOrUpdateSFTPUser(mockService, nil, nil)
		expectUpdatePublicKeysForSFTPUser(mockService, &keys, nil)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())

		Expect(keys).To(HaveKey("items"))
		Expect(keys["items"]).To(ConsistOf(service.RoverPublicKeyModel{
			PublicKey:    "ssh-rsa cHJvdmlkZXI=",
			SftpUserName: instanceHandlerTestName,
			Description:  ptrTo(instanceHandlerTestNamespace + "/" + instanceHandlerTestUserName),
		}))
		Expect(instance.Status.Users).To(HaveLen(1))
		userStatus := instance.Status.Users[0]
		Expect(userStatus.Namespace).To(Equal(instanceHandlerTestNamespace))
		Expect(userStatus.Name).To(Equal(instanceHandlerTestUserName))
		Expect(userStatus.ProcessingCondition.Type).To(Equal(condition.ConditionTypeProcessing))
		Expect(userStatus.ProcessingCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(userStatus.ProcessingCondition.Reason).To(Equal("Done"))
		Expect(userStatus.ProcessingCondition.ObservedGeneration).To(Equal(user.Generation))
		Expect(userStatus.ProcessingCondition.LastTransitionTime.IsZero()).To(BeFalse())
	})

	It("syncs keys only from users referencing the Instance", func() {
		publicKey := "ssh-rsa cHJvdmlkZXI= provider@example.com"
		unrelatedKey := "ssh-rsa dW5yZWxhdGVk unrelated@example.com"

		user := testUser()
		user.Spec.SSHPublicKeys = []string{publicKey}

		unrelatedUser := testUserNamed("unrelated-user")
		unrelatedUser.Spec.InstanceRef.Name = "other-instance"
		unrelatedUser.Spec.SSHPublicKeys = []string{unrelatedKey}

		handler, ctx, instance, mockService := newTestHandler(user, unrelatedUser)
		var keys service.ClientPublicKeyMap
		expectCreateOrUpdateSFTPUser(mockService, nil, nil)
		expectUpdatePublicKeysForSFTPUser(mockService, &keys, nil)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())

		Expect(keys["items"]).To(ConsistOf(service.RoverPublicKeyModel{
			PublicKey:    "ssh-rsa cHJvdmlkZXI=",
			SftpUserName: instanceHandlerTestName,
			Description:  ptrTo(instanceHandlerTestNamespace + "/" + instanceHandlerTestUserName),
		}))
	})

	It("syncs keys only from users in the scoped environment", func() {
		publicKey := "ssh-rsa cHJvdmlkZXI= provider@example.com"
		otherEnvironmentKey := "ssh-rsa b3RoZXItZW52 other-env@example.com"

		user := testUser()
		user.Spec.SSHPublicKeys = []string{publicKey}

		otherEnvironmentUser := testUserNamed("other-environment-user")
		otherEnvironmentUser.Labels = map[string]string{
			config.EnvironmentLabelKey: "other",
		}
		otherEnvironmentUser.Spec.SSHPublicKeys = []string{otherEnvironmentKey}

		handler, ctx, instance, mockService := newTestHandler(user, otherEnvironmentUser)
		var keys service.ClientPublicKeyMap
		expectCreateOrUpdateSFTPUser(mockService, nil, nil)
		expectUpdatePublicKeysForSFTPUser(mockService, &keys, nil)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())

		Expect(keys["items"]).To(ConsistOf(service.RoverPublicKeyModel{
			PublicKey:    "ssh-rsa cHJvdmlkZXI=",
			SftpUserName: instanceHandlerTestName,
			Description:  ptrTo(instanceHandlerTestNamespace + "/" + instanceHandlerTestUserName),
		}))
	})

	It("clears DDS keys when the user has no SSH public keys", func() {
		handler, ctx, instance, mockService := newTestHandler(testUser())
		var keys service.ClientPublicKeyMap
		expectCreateOrUpdateSFTPUser(mockService, nil, nil)
		expectUpdatePublicKeysForSFTPUser(mockService, &keys, nil)

		Expect(handler.CreateOrUpdate(ctx, instance)).To(Succeed())

		Expect(keys).To(HaveKey("items"))
		Expect(keys["items"]).To(BeEmpty())
	})

	It("marks public keys as not updated when service key synchronization fails", func() {
		handler, ctx, instance, mockService := newTestHandler(testUser())
		expectCreateOrUpdateSFTPUser(mockService, nil, nil)
		expectUpdatePublicKeysForSFTPUser(mockService, nil, errors.New("dds unavailable"))

		err := handler.CreateOrUpdate(ctx, instance)

		Expect(err).To(MatchError(ContainSubstring("updating public keys for SFTP user")))
		Expect(err).To(MatchError(ContainSubstring("dds unavailable")))
		Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, sftpv1.ConditionTypePublicKeysUpdatedInService)).To(BeTrue())
		Expect(instance.Status.Users).To(BeEmpty())
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
		handler, ctx, instance, _ := newTestHandler()
		handler.ServiceFactory = recordingFactory{err: errors.New("service unavailable")}

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

	It("collects unique public keys and keeps the highest user description", func() {
		publicKey := "ssh-rsa cHJvdmlkZXI= provider@example.com"
		users := []sftpv1.User{
			*testUserNamed("middle-user"),
			*testUserNamed("z-user"),
			*testUserNamed("a-user"),
		}
		for i := range users {
			users[i].Spec.SSHPublicKeys = []string{publicKey}
		}

		keys, userStatuses := collectUniquePublicKeysFromUsers(GinkgoLogr, instanceHandlerTestName, users)

		Expect(keys).To(ConsistOf(service.RoverPublicKeyModel{
			PublicKey:    "ssh-rsa cHJvdmlkZXI=",
			SftpUserName: instanceHandlerTestName,
			Description:  ptrTo(instanceHandlerTestNamespace + "/z-user"),
		}))
		Expect(userStatuses).To(HaveLen(3))
		Expect(userStatuses[0].ProcessingCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(userStatuses[0].ProcessingCondition.Reason).To(Equal("Done"))
	})

	It("ignores invalid public keys while collecting service payload", func() {
		users := []sftpv1.User{*testUser()}
		users[0].Spec.SSHPublicKeys = []string{
			"invalid",
			"ssh-rsa not-base64",
		}

		keys, userStatuses := collectUniquePublicKeysFromUsers(GinkgoLogr, instanceHandlerTestName, users)

		Expect(keys).To(BeEmpty())
		Expect(userStatuses).To(HaveLen(1))
		Expect(userStatuses[0].ProcessingCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(userStatuses[0].ProcessingCondition.Reason).To(Equal("Blocked"))
		Expect(userStatuses[0].ProcessingCondition.Message).To(ContainSubstring("Failed to process public key"))
		Expect(userStatuses[0].ProcessingCondition.LastTransitionTime.IsZero()).To(BeFalse())
	})
})

func newTestHandler(objects ...client.Object) (*InstanceHandler, context.Context, *sftpv1.Instance, *sftpmocks.MockService) {
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(sftpv1.AddToScheme(scheme)).To(Succeed())

	sftpServiceConfig := &sftpv1.SFTPServiceConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: sftpv1.GroupVersion.String(),
			Kind:       "SFTPServiceConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceHandlerTestSFTPServiceConfigName,
			Namespace: instanceHandlerTestNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: instanceHandlerTestEnvironment,
			},
		},
	}
	instance := testInstance()
	mockService := sftpmocks.NewMockService(GinkgoT())

	allObjects := append([]client.Object{sftpServiceConfig, instance}, objects...)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&sftpv1.Instance{}, &sftpv1.User{}).
		WithObjects(allObjects...).
		WithIndex(&sftpv1.User{}, sftpv1.IndexFieldSpecInstanceRef, func(obj client.Object) []string {
			user, ok := obj.(*sftpv1.User)
			if !ok || user.Spec.InstanceRef.IsEmpty() {
				return nil
			}
			return []string{user.Spec.InstanceRef.String()}
		}).
		Build()

	handler := &InstanceHandler{
		Client:         k8sClient,
		ServiceFactory: recordingFactory{svc: mockService},
	}
	ctx := cclient.WithClient(
		context.Background(),
		cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, instanceHandlerTestEnvironment)),
	)
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
			Labels: map[string]string{
				config.EnvironmentLabelKey: instanceHandlerTestEnvironment,
			},
		},
		Spec: sftpv1.InstanceSpec{
			SFTPServiceConfigRef: types.ObjectRef{
				Name:      instanceHandlerTestSFTPServiceConfigName,
				Namespace: instanceHandlerTestNamespace,
			},
		},
	}
}

func testUser() *sftpv1.User {
	return testUserNamed(instanceHandlerTestUserName)
}

func testUserNamed(name string) *sftpv1.User {
	return &sftpv1.User{
		TypeMeta: metav1.TypeMeta{
			APIVersion: sftpv1.GroupVersion.String(),
			Kind:       "User",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  instanceHandlerTestNamespace,
			Generation: 1,
			Labels: map[string]string{
				config.EnvironmentLabelKey: instanceHandlerTestEnvironment,
			},
		},
		Spec: sftpv1.UserSpec{
			InstanceRef: types.ObjectRef{
				Name:      instanceHandlerTestName,
				Namespace: instanceHandlerTestNamespace,
			},
		},
	}
}

func ptrTo[T any](value T) *T {
	return &value
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

func expectUpdatePublicKeysForSFTPUser(mockService *sftpmocks.MockService, capturedKeys *service.ClientPublicKeyMap, err error) {
	call := mockService.EXPECT().UpdatePublicKeysForSFTPUser(mock.Anything, instanceHandlerTestName, instanceHandlerTestName, mock.Anything)
	if capturedKeys != nil {
		call.Run(func(_ context.Context, _ string, _ string, keys service.ClientPublicKeyMap) {
			*capturedKeys = keys
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
