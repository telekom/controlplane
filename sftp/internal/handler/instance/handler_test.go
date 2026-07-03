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

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	"github.com/telekom/controlplane/sftp/internal/service"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	instanceHandlerTestNamespace             = "test"
	instanceHandlerTestName                  = "test-instance"
	instanceHandlerTestSFTPServiceConfigName = "test-sftpServiceConfig"
	instanceHandlerTestUserName              = "test-user"
)

var _ = Describe("InstanceHandler", func() {
	It("marks an Instance ready when its SFTPServiceConfig exists", func() {
		handler, instance, _ := newTestHandler()

		Expect(handler.CreateOrUpdate(context.Background(), instance)).To(Succeed())
		Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, condition.ConditionTypeProcessing)).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, sftpv1.ConditionTypePublicKeysUpdatedInService)).To(BeTrue())
	})

	It("blocks when SFTPServiceConfig reference is missing", func() {
		handler, instance, _ := newTestHandler()
		instance.Spec.SFTPServiceConfigRef = types.ObjectRef{}

		err := handler.CreateOrUpdate(context.Background(), instance)

		var blocked ctrlerrors.BlockedError
		Expect(errors.As(err, &blocked)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring("SFTPServiceConfig reference is required")))
	})

	It("retries when the referenced SFTPServiceConfig service is unavailable", func() {
		handler, instance, _ := newTestHandler()
		handler.ServiceFactory = recordingFactory{
			err: ctrlerrors.RetryableErrorf("SFTP client for SFTPServiceConfig %q is not initialized", "test/test-sftpServiceConfig"),
		}

		err := handler.CreateOrUpdate(context.Background(), instance)

		var retryable ctrlerrors.RetryableError
		Expect(errors.As(err, &retryable)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring("SFTPServiceConfig")))
		Expect(err).To(MatchError(ContainSubstring("not initialized")))
	})

	It("creates a service user with description and empty Horizon notification events", func() {
		handler, instance, recorder := newTestHandler()
		instance.Spec.Description = "Team transfer user"

		Expect(handler.CreateOrUpdate(context.Background(), instance)).To(Succeed())

		Expect(recorder.createdModels).To(HaveLen(1))
		Expect(recorder.createdModels[0].SftpUserName).To(Equal(instanceHandlerTestName))
		Expect(recorder.createdModels[0].Description).NotTo(BeNil())
		Expect(*recorder.createdModels[0].Description).To(Equal("Team transfer user"))
		Expect(recorder.createdModels[0].HorizonNotificationEvents).NotTo(BeNil())
		Expect(*recorder.createdModels[0].HorizonNotificationEvents).To(BeEmpty())
	})

	It("skips service user provisioning when the Ready condition observed generation is current", func() {
		handler, instance, recorder := newTestHandler()
		ready := condition.NewReadyCondition("InstanceProvided", "Instance has been provided")
		ready.ObservedGeneration = instance.Generation
		instance.SetCondition(ready)

		Expect(handler.CreateOrUpdate(context.Background(), instance)).To(Succeed())

		Expect(recorder.createCalls).To(Equal(0))
		Expect(recorder.updateCalls).To(Equal(1))
	})

	It("provisions the service user when the Ready condition observed generation is stale", func() {
		handler, instance, recorder := newTestHandler()
		ready := condition.NewReadyCondition("InstanceProvided", "Instance has been provided")
		ready.ObservedGeneration = instance.Generation - 1
		instance.SetCondition(ready)

		Expect(handler.CreateOrUpdate(context.Background(), instance)).To(Succeed())

		Expect(recorder.createCalls).To(Equal(1))
		Expect(recorder.updateCalls).To(Equal(1))
	})

	It("returns service user creation errors", func() {
		handler, instance, recorder := newTestHandler()
		recorder.createErr = errors.New("create failed")

		err := handler.CreateOrUpdate(context.Background(), instance)

		Expect(err).To(MatchError(ContainSubstring("creating or updating SFTP user")))
		Expect(err).To(MatchError(ContainSubstring("create failed")))
		Expect(recorder.updateCalls).To(Equal(0))
	})

	It("syncs user SSH public keys to DDS grouped by instance name", func() {
		publicKey := "ssh-rsa cHJvdmlkZXI= provider@example.com"

		user := testUser()
		user.Spec.SSHPublicKeys = []string{publicKey}

		handler, instance, recorder := newTestHandler(user)

		Expect(handler.CreateOrUpdate(context.Background(), instance)).To(Succeed())

		Expect(recorder.createCalls).To(Equal(1))
		Expect(recorder.createdUsers).To(ConsistOf(instanceHandlerTestName))
		Expect(recorder.updateCalls).To(Equal(1))
		Expect(recorder.sftpUserName).To(Equal(instanceHandlerTestName))
		Expect(recorder.clientID).To(Equal(instanceHandlerTestName))
		Expect(recorder.keys).To(HaveKey("items"))
		Expect(recorder.keys["items"]).To(ConsistOf(service.RoverPublicKeyModel{
			PublicKey:    "ssh-rsa cHJvdmlkZXI=",
			SftpUserName: instanceHandlerTestName,
			Description:  ptrTo(instanceHandlerTestNamespace + "/" + instanceHandlerTestUserName),
		}))
	})

	It("syncs keys only from users referencing the Instance", func() {
		publicKey := "ssh-rsa cHJvdmlkZXI= provider@example.com"
		unrelatedKey := "ssh-rsa dW5yZWxhdGVk unrelated@example.com"

		user := testUser()
		user.Spec.SSHPublicKeys = []string{publicKey}

		unrelatedUser := testUserNamed("unrelated-user")
		unrelatedUser.Spec.InstanceRef.Name = "other-instance"
		unrelatedUser.Spec.SSHPublicKeys = []string{unrelatedKey}

		handler, instance, recorder := newTestHandler(user, unrelatedUser)

		Expect(handler.CreateOrUpdate(context.Background(), instance)).To(Succeed())

		Expect(recorder.keys["items"]).To(ConsistOf(service.RoverPublicKeyModel{
			PublicKey:    "ssh-rsa cHJvdmlkZXI=",
			SftpUserName: instanceHandlerTestName,
			Description:  ptrTo(instanceHandlerTestNamespace + "/" + instanceHandlerTestUserName),
		}))
	})

	It("clears DDS keys when the user has no SSH public keys", func() {
		handler, instance, recorder := newTestHandler(testUser())

		Expect(handler.CreateOrUpdate(context.Background(), instance)).To(Succeed())

		Expect(recorder.updateCalls).To(Equal(1))
		Expect(recorder.clientID).To(Equal(instanceHandlerTestName))
		Expect(recorder.keys).To(HaveKey("items"))
		Expect(recorder.keys["items"]).To(BeEmpty())
	})

	It("marks public keys as not updated when service key synchronization fails", func() {
		handler, instance, recorder := newTestHandler()
		recorder.updateErr = errors.New("dds unavailable")

		err := handler.CreateOrUpdate(context.Background(), instance)

		Expect(err).To(MatchError(ContainSubstring("updating public keys for SFTP user")))
		Expect(err).To(MatchError(ContainSubstring("dds unavailable")))
		Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, sftpv1.ConditionTypePublicKeysUpdatedInService)).To(BeTrue())
	})

	It("deletes the instance SFTP user", func() {
		handler, instance, recorder := newTestHandler()

		Expect(handler.Delete(context.Background(), instance)).To(Succeed())

		Expect(recorder.deletedUsers).To(ConsistOf(instanceHandlerTestName))
	})

	It("does not delete a service user when SFTPServiceConfig reference is missing", func() {
		handler, instance, recorder := newTestHandler()
		instance.Spec.SFTPServiceConfigRef = types.ObjectRef{}

		Expect(handler.Delete(context.Background(), instance)).To(Succeed())

		Expect(recorder.deletedUsers).To(BeEmpty())
	})

	It("returns service lookup errors during deletion", func() {
		handler, instance, _ := newTestHandler()
		handler.ServiceFactory = recordingFactory{err: errors.New("service unavailable")}

		err := handler.Delete(context.Background(), instance)

		Expect(err).To(MatchError("service unavailable"))
	})

	It("wraps service user deletion errors", func() {
		handler, instance, recorder := newTestHandler()
		recorder.deleteErr = errors.New("delete failed")

		err := handler.Delete(context.Background(), instance)

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

		keys := collectUniquePublicKeysFromUsers(GinkgoLogr, instanceHandlerTestName, users)

		Expect(keys).To(ConsistOf(service.RoverPublicKeyModel{
			PublicKey:    "ssh-rsa cHJvdmlkZXI=",
			SftpUserName: instanceHandlerTestName,
			Description:  ptrTo(instanceHandlerTestNamespace + "/z-user"),
		}))
		Expect(meta.IsStatusConditionFalse(users[0].Status.Conditions, condition.ConditionTypeProcessing)).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(users[0].Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
	})

	It("ignores invalid public keys while collecting service payload", func() {
		users := []sftpv1.User{*testUser()}
		users[0].Spec.SSHPublicKeys = []string{
			"invalid",
			"ssh-rsa not-base64",
		}

		keys := collectUniquePublicKeysFromUsers(GinkgoLogr, instanceHandlerTestName, users)

		Expect(keys).To(BeEmpty())
		ready := meta.FindStatusCondition(users[0].Status.Conditions, condition.ConditionTypeReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Message).To(ContainSubstring("Failed to process public key"))
	})

	It("returns joined errors when updating user status fails for one user", func() {
		existingUser := testUser()
		missingUser := testUserNamed("missing-user")
		handler, _, _ := newTestHandler(existingUser)

		err := handler.updateUserStatus(context.Background(), []sftpv1.User{*existingUser, *missingUser})

		Expect(err).To(MatchError(ContainSubstring("updating status for User")))
		Expect(err).To(MatchError(ContainSubstring("missing-user")))
	})
})

func newTestHandler(objects ...client.Object) (*InstanceHandler, *sftpv1.Instance, *recordingService) {
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
		},
	}
	instance := testInstance()
	recorder := &recordingService{}

	allObjects := append([]client.Object{sftpServiceConfig, instance}, objects...)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&sftpv1.Instance{}, &sftpv1.User{}).
		WithObjects(allObjects...).
		Build()

	handler := &InstanceHandler{
		Client:         k8sClient,
		ServiceFactory: recordingFactory{svc: recorder},
	}
	return handler, instance, recorder
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

type recordingFactory struct {
	svc service.Service
	err error
}

func (f recordingFactory) ServiceFor(context.Context, client.ObjectKey) (service.Service, error) {
	return f.svc, f.err
}

type recordingService struct {
	createCalls   int
	updateCalls   int
	createdUsers  []string
	createdModels []service.RoverSftpUserModel
	sftpUserName  string
	clientID      string
	keys          service.ClientPublicKeyMap
	deletedUsers  []string
	createErr     error
	updateErr     error
	deleteErr     error
}

func (s *recordingService) CreateOrUpdateSFTPUser(_ context.Context, user service.RoverSftpUserModel) error {
	s.createCalls++
	s.createdUsers = append(s.createdUsers, user.SftpUserName)
	s.createdModels = append(s.createdModels, user)
	return s.createErr
}

func (s *recordingService) UpdatePublicKeysForSFTPUser(_ context.Context, sftpUserName, clientID string, keys service.ClientPublicKeyMap) error {
	s.updateCalls++
	s.sftpUserName = sftpUserName
	s.clientID = clientID
	s.keys = keys
	return s.updateErr
}

func (s *recordingService) DeleteSFTPUser(_ context.Context, sftpUserName string) error {
	s.deletedUsers = append(s.deletedUsers, sftpUserName)
	return s.deleteErr
}
