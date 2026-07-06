// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package user

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	userHandlerTestEnvironment = "test"
	userHandlerTestNamespace   = "test"
	userHandlerTestInstance    = "test-instance"
	userHandlerTestName        = "test-user"
)

var _ = Describe("UserHandler", func() {
	It("mirrors completed processing status from the Instance and marks the User ready", func() {
		instance := testInstanceWithReadyStatus()
		user := testUser()
		instance.Status.Users = []sftpv1.InstanceUserStatus{
			testInstanceUserStatus(user, condition.NewDoneProcessingCondition("SSH public keys were processed")),
		}
		handler, ctx := newTestHandler(instance, user)

		Expect(handler.CreateOrUpdate(ctx, user)).To(Succeed())

		Expect(meta.IsStatusConditionFalse(user.Status.Conditions, condition.ConditionTypeProcessing)).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(user.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		ready := meta.FindStatusCondition(user.Status.Conditions, condition.ConditionTypeReady)
		Expect(ready.Reason).To(Equal(sftpv1.ConditionReadyReasonSSHPublicKeyProvided))
	})

	It("marks the User not ready when the Instance reports blocked processing", func() {
		instance := testInstanceWithReadyStatus()
		user := testUser()
		instance.Status.Users = []sftpv1.InstanceUserStatus{
			testInstanceUserStatus(user, condition.NewBlockedCondition("Failed to process public key")),
		}
		handler, ctx := newTestHandler(instance, user)

		Expect(handler.CreateOrUpdate(ctx, user)).To(Succeed())

		processing := meta.FindStatusCondition(user.Status.Conditions, condition.ConditionTypeProcessing)
		Expect(processing).NotTo(BeNil())
		Expect(processing.Status).To(Equal(metav1.ConditionFalse))
		Expect(processing.Reason).To(Equal("Blocked"))
		ready := meta.FindStatusCondition(user.Status.Conditions, condition.ConditionTypeReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Message).To(Equal("Failed to process public key"))
	})

	It("keeps the User processing while the Instance has no status for it", func() {
		instance := testInstanceWithReadyStatus()
		user := testUser()
		handler, ctx := newTestHandler(instance, user)

		Expect(handler.CreateOrUpdate(ctx, user)).To(Succeed())

		Expect(meta.IsStatusConditionTrue(user.Status.Conditions, condition.ConditionTypeProcessing)).To(BeTrue())
		Expect(meta.IsStatusConditionFalse(user.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
	})

	It("keeps the User processing while the Instance user status is stale", func() {
		instance := testInstanceWithReadyStatus()
		user := testUser()
		user.Generation = 2
		staleStatus := testInstanceUserStatus(user, condition.NewDoneProcessingCondition("SSH public keys were processed"))
		staleStatus.ProcessingCondition.ObservedGeneration = user.Generation - 1
		instance.Status.Users = []sftpv1.InstanceUserStatus{staleStatus}
		handler, ctx := newTestHandler(instance, user)

		Expect(handler.CreateOrUpdate(ctx, user)).To(Succeed())

		processing := meta.FindStatusCondition(user.Status.Conditions, condition.ConditionTypeProcessing)
		Expect(processing).NotTo(BeNil())
		Expect(processing.Status).To(Equal(metav1.ConditionTrue))
		Expect(processing.Reason).To(Equal("WaitingForInstance"))
		Expect(meta.IsStatusConditionFalse(user.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
	})

	It("keeps the User processing while the Instance is not ready", func() {
		instance := testInstance()
		user := testUser()
		instance.Status.Users = []sftpv1.InstanceUserStatus{
			testInstanceUserStatus(user, condition.NewDoneProcessingCondition("SSH public keys were processed")),
		}
		handler, ctx := newTestHandler(instance, user)

		Expect(handler.CreateOrUpdate(ctx, user)).To(Succeed())

		processing := meta.FindStatusCondition(user.Status.Conditions, condition.ConditionTypeProcessing)
		Expect(processing).NotTo(BeNil())
		Expect(processing.Status).To(Equal(metav1.ConditionTrue))
		Expect(processing.Message).To(Equal("Waiting for Instance to be ready"))
		Expect(meta.IsStatusConditionFalse(user.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
	})

	It("blocks when the Instance reference is missing", func() {
		instance := testInstanceWithReadyStatus()
		user := testUser()
		user.Spec.InstanceRef = types.ObjectRef{}
		handler, ctx := newTestHandler(instance, user)

		err := handler.CreateOrUpdate(ctx, user)

		var blocked ctrlerrors.BlockedError
		Expect(errors.As(err, &blocked)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring("Instance reference is required")))
	})
})

func newTestHandler(objects ...client.Object) (*UserHandler, context.Context) {
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(sftpv1.AddToScheme(scheme)).To(Succeed())

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&sftpv1.Instance{}, &sftpv1.User{}).
		WithObjects(objects...).
		Build()

	ctx := cclient.WithClient(
		context.Background(),
		cclient.NewJanitorClient(cclient.NewScopedClient(k8sClient, userHandlerTestEnvironment)),
	)
	return New(), ctx
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
			Labels: map[string]string{
				config.EnvironmentLabelKey: userHandlerTestEnvironment,
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
	return &sftpv1.User{
		TypeMeta: metav1.TypeMeta{
			APIVersion: sftpv1.GroupVersion.String(),
			Kind:       "User",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       userHandlerTestName,
			Namespace:  userHandlerTestNamespace,
			Generation: 1,
			Labels: map[string]string{
				config.EnvironmentLabelKey: userHandlerTestEnvironment,
			},
		},
		Spec: sftpv1.UserSpec{
			InstanceRef: types.ObjectRef{
				Name:      userHandlerTestInstance,
				Namespace: userHandlerTestNamespace,
			},
		},
	}
}

//nolint:gocritic // it is used with tests, extra optimization is not needed
func testInstanceUserStatus(user *sftpv1.User, processing metav1.Condition) sftpv1.InstanceUserStatus {
	processing.ObservedGeneration = user.Generation
	processing.LastTransitionTime = metav1.Now()
	return sftpv1.InstanceUserStatus{
		Namespace:           user.Namespace,
		Name:                user.Name,
		ProcessingCondition: processing,
	}
}
