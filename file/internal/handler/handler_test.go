// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"

	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	controllerindex "github.com/telekom/controlplane/common/pkg/controller/index"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/file/internal/handler/fileexposure"
	"github.com/telekom/controlplane/file/internal/handler/filesubscription"
	"github.com/telekom/controlplane/file/internal/handler/filetype"
	handlerutil "github.com/telekom/controlplane/file/internal/handler/util"
	"github.com/telekom/controlplane/file/internal/handler/zoneserviceconfig"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const testEnvironment = "test"

var _ = Describe("File handlers", func() {
	Describe("ZoneServiceConfigHandler", func() {
		It("projects file ZoneServiceConfig to the SFTP domain", func() {
			obj := newZoneServiceConfig("zone-a")
			ctx, k8sClient := newHandlerContext(obj)

			err := (&zoneserviceconfig.ZoneServiceConfigHandler{}).CreateOrUpdate(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			sftpConfig := &sftpv1.ZoneServiceConfig{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "zone-a", Namespace: "team-a"}, sftpConfig)).To(Succeed())
			Expect(sftpConfig.Spec.API.Endpoint).To(Equal("https://sftp-api.zone-a.example.com"))
			Expect(sftpConfig.Spec.API.Issuer).To(Equal("https://issuer.zone-a.example.com/token"))
			Expect(sftpConfig.Spec.Service.Host).To(Equal("sftp.zone-a.svc"))
			Expect(sftpConfig.Spec.ServiceExternal.Host).To(Equal("sftp.zone-a.example.com"))
			Expect(obj.Status.SFTPZoneServiceConfigRef).To(Equal(ctypes.ObjectRefFromObject(sftpConfig)))
			Expect(obj.GetConditions()).To(haveReadyCondition())
		})
	})

	Describe("FileTypeHandler", func() {
		It("creates an SFTP User from the active FileExposure", func() {
			fileTypeObj := newFileType("orders")
			zoneConfig := newZoneServiceConfig("zone-a")
			exposure := newFileExposure("orders-provider", fileTypeObj, zoneConfig)
			exposure.Spec.SFTP.PublicKeys = []filev1.SSHPublicKeySpec{{
				Key: "ssh-rsa cHJvdmlkZXI= provider@example.com",
			}}
			ctx, k8sClient := newHandlerContext(fileTypeObj, zoneConfig, exposure)

			err := (&filetype.FileTypeHandler{}).CreateOrUpdate(ctx, fileTypeObj)

			Expect(err).NotTo(HaveOccurred())
			user := &sftpv1.User{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "orders", Namespace: "team-a"}, user)).To(Succeed())
			Expect(user.Spec.Description).To(Equal("Orders data"))
			Expect(user.Spec.InstanceRef).To(Equal(ctypes.ObjectRef{Name: "orders", Namespace: "team-a"}))
			Expect(user.Spec.SSHPublicKeys).To(ConsistOf("ssh-rsa cHJvdmlkZXI="))
			Expect(fileTypeObj.Status.FileExposureRef).To(Equal(ctypes.ObjectRefFromObject(exposure)))
			Expect(fileTypeObj.Status.SFTPUserRef).To(Equal(ctypes.ObjectRefFromObject(user)))
			Expect(fileTypeObj.GetConditions()).To(haveReadyCondition())
		})

		It("deletes the projected SFTP User", func() {
			fileTypeObj := newFileType("orders")
			user := &sftpv1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "orders",
					Namespace: "team-a",
				},
			}
			fileTypeObj.Status.SFTPUserRef = ctypes.ObjectRefFromObject(user)
			ctx, k8sClient := newHandlerContext(fileTypeObj, user)

			err := (&filetype.FileTypeHandler{}).Delete(ctx, fileTypeObj)

			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "orders", Namespace: "team-a"}, &sftpv1.User{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("ignores missing projected SFTP Users during deletion", func() {
			fileTypeObj := newFileType("orders")
			ctx, _ := newHandlerContext(fileTypeObj)

			err := (&filetype.FileTypeHandler{}).Delete(ctx, fileTypeObj)

			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("FileExposureHandler", func() {
		It("creates an SFTP Instance for the active FileExposure", func() {
			fileTypeObj := newFileType("orders")
			zoneConfig := newZoneServiceConfig("zone-a")
			exposure := newFileExposure("orders-provider", fileTypeObj, zoneConfig)
			ctx, k8sClient := newHandlerContext(fileTypeObj, zoneConfig, exposure)

			err := (&fileexposure.FileExposureHandler{}).CreateOrUpdate(ctx, exposure)

			Expect(err).NotTo(HaveOccurred())
			instance := &sftpv1.Instance{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "orders", Namespace: "team-a"}, instance)).To(Succeed())
			Expect(instance.Spec.Description).To(Equal("Orders data"))
			Expect(instance.Spec.ZoneServiceConfigRef).To(Equal(*ctypes.ObjectRefFromObject(zoneConfig)))
			Expect(exposure.GetConditions()).To(haveReadyCondition())
		})
	})

	Describe("FileSubscriptionHandler", func() {
		It("creates subscriber SFTP public keys after approval is granted", func() {
			fileTypeObj := newFileType("orders")
			zoneConfig := newZoneServiceConfig("zone-a")
			exposure := newFileExposure("orders-provider", fileTypeObj, zoneConfig)
			activateFileExposure(fileTypeObj, exposure)
			subscription := newFileSubscription("orders-consumer", fileTypeObj)
			publicKey := "ssh-rsa Y29uc3VtZXI= consumer@example.com"
			subscription.Spec.SFTP.PublicKeys = []filev1.SSHPublicKeySpec{{
				Key: publicKey,
			}}
			ctx, k8sClient := newHandlerContext(fileTypeObj, zoneConfig, exposure, subscription)
			handler := &filesubscription.FileSubscriptionHandler{}

			err := handler.CreateOrUpdate(ctx, subscription)
			Expect(err).NotTo(HaveOccurred())
			Expect(subscription.GetConditions()).To(haveNotReadyCondition("ApprovalPending"))
			grantFileSubscriptionApproval(ctx, k8sClient, subscription)

			err = handler.CreateOrUpdate(ctx, subscription)

			Expect(err).NotTo(HaveOccurred())
			user := &sftpv1.User{}
			subscriberUserRef := handlerutil.SFTPUserRefForFileSubscription(subscription)
			Expect(k8sClient.Get(ctx, subscriberUserRef.K8s(), user)).To(Succeed())
			Expect(user.Spec.InstanceRef).To(Equal(ctypes.ObjectRef{Name: "orders", Namespace: "team-a"}))
			Expect(user.Spec.SSHPublicKeys).To(ConsistOf("ssh-rsa Y29uc3VtZXI="))
			Expect(subscription.GetConditions()).To(haveReadyCondition())
		})

		It("does not create subscriber SFTP public keys while approval is pending", func() {
			fileTypeObj := newFileType("orders")
			zoneConfig := newZoneServiceConfig("zone-a")
			exposure := newFileExposure("orders-provider", fileTypeObj, zoneConfig)
			activateFileExposure(fileTypeObj, exposure)
			subscription := newFileSubscription("orders-consumer", fileTypeObj)
			subscription.Spec.SFTP.PublicKeys = []filev1.SSHPublicKeySpec{{
				Key: "ssh-rsa Y29uc3VtZXI= consumer@example.com",
			}}
			ctx, k8sClient := newHandlerContext(fileTypeObj, zoneConfig, exposure, subscription)

			err := (&filesubscription.FileSubscriptionHandler{}).CreateOrUpdate(ctx, subscription)

			Expect(err).NotTo(HaveOccurred())
			Expect(subscription.Status.ApprovalRequest).ToNot(BeNil())
			Expect(subscription.Status.Approval).ToNot(BeNil())
			Expect(subscription.GetConditions()).To(haveNotReadyCondition("ApprovalPending"))
			subscriberUserRef := handlerutil.SFTPUserRefForFileSubscription(subscription)
			err = k8sClient.Get(ctx, subscriberUserRef.K8s(), &sftpv1.User{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("removes subscriber SFTP public keys when approval is denied", func() {
			fileTypeObj := newFileType("orders")
			zoneConfig := newZoneServiceConfig("zone-a")
			exposure := newFileExposure("orders-provider", fileTypeObj, zoneConfig)
			activateFileExposure(fileTypeObj, exposure)
			subscription := newFileSubscription("orders-consumer", fileTypeObj)
			subscription.Spec.SFTP.PublicKeys = []filev1.SSHPublicKeySpec{{
				Key: "ssh-rsa Y29uc3VtZXI= consumer@example.com",
			}}
			ctx, k8sClient := newHandlerContext(fileTypeObj, zoneConfig, exposure, subscription)
			handler := &filesubscription.FileSubscriptionHandler{}

			err := handler.CreateOrUpdate(ctx, subscription)
			Expect(err).NotTo(HaveOccurred())
			approval := grantFileSubscriptionApproval(ctx, k8sClient, subscription)
			err = handler.CreateOrUpdate(ctx, subscription)
			Expect(err).NotTo(HaveOccurred())

			approval.Spec.State = approvalv1.ApprovalStateRejected
			Expect(k8sClient.Update(ctx, approval)).To(Succeed())
			err = handler.CreateOrUpdate(ctx, subscription)

			Expect(err).NotTo(HaveOccurred())
			subscriberUserRef := handlerutil.SFTPUserRefForFileSubscription(subscription)
			err = k8sClient.Get(ctx, subscriberUserRef.K8s(), &sftpv1.User{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			Expect(subscription.GetConditions()).To(haveNotReadyCondition("ApprovalDenied"))
		})

		It("blocks zone-restricted subscriptions from another zone", func() {
			fileTypeObj := newFileType("orders")
			zoneConfig := newZoneServiceConfig("zone-a")
			exposure := newFileExposure("orders-provider", fileTypeObj, zoneConfig)
			activateFileExposure(fileTypeObj, exposure)
			exposure.Spec.Visibility = filev1.VisibilityZone
			exposure.Spec.Zone = &ctypes.ObjectRef{Name: "zone-a", Namespace: "team-a"}
			subscription := newFileSubscription("orders-consumer", fileTypeObj)
			subscription.Spec.Zone = &ctypes.ObjectRef{Name: "zone-b", Namespace: "team-a"}
			ctx, _ := newHandlerContext(fileTypeObj, zoneConfig, exposure, subscription)

			err := (&filesubscription.FileSubscriptionHandler{}).CreateOrUpdate(ctx, subscription)

			Expect(err).To(HaveOccurred())
			Expect(subscription.GetConditions()).To(haveNotReadyCondition("VisibilityConstraintViolation"))
		})
	})
})

func newHandlerContext(objs ...client.Object) (context.Context, client.Client) {
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(approvalv1.AddToScheme(scheme)).To(Succeed())
	Expect(filev1.AddToScheme(scheme)).To(Succeed())
	Expect(sftpv1.AddToScheme(scheme)).To(Succeed())

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team-a"}}
	allObjects := []client.Object{ns}
	for i := range objs {
		ensureEnvironmentLabel(objs[i])
		allObjects = append(allObjects, objs[i])
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&approvalv1.ApprovalRequest{}, controllerindex.ControllerIndexKey, controllerOwnerIndex).
		WithObjects(allObjects...).
		Build()
	scopedClient := cclient.NewScopedClient(k8sClient, testEnvironment)
	ctx := cclient.WithClient(context.Background(), cclient.NewJanitorClient(scopedClient))
	return ctx, k8sClient
}

func controllerOwnerIndex(obj client.Object) []string {
	owner := metav1.GetControllerOf(obj)
	if owner == nil {
		return nil
	}
	return []string{string(owner.UID)}
}

func ensureEnvironmentLabel(obj client.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[config.EnvironmentLabelKey] = testEnvironment
	obj.SetLabels(labels)
}

func grantFileSubscriptionApproval(ctx context.Context, k8sClient client.Client, subscription *filev1.FileSubscription) *approvalv1.Approval {
	approvalReq := &approvalv1.ApprovalRequest{}
	Expect(k8sClient.Get(ctx, subscription.Status.ApprovalRequest.K8s(), approvalReq)).To(Succeed())

	approval := &approvalv1.Approval{
		ObjectMeta: metav1.ObjectMeta{
			Name:      subscription.Status.Approval.Name,
			Namespace: subscription.Status.Approval.Namespace,
			UID:       ktypes.UID("approval-" + subscription.Name),
		},
		Spec: approvalv1.ApprovalSpec{
			State:           approvalv1.ApprovalStateGranted,
			ApprovedRequest: ctypes.ObjectRefFromObject(approvalReq),
		},
	}
	ensureEnvironmentLabel(approval)
	Expect(k8sClient.Create(ctx, approval)).To(Succeed())
	return approval
}

func newFileType(name string) *filev1.FileType {
	return &filev1.FileType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "team-a",
			UID:       ktypes.UID("filetype-" + name),
		},
		Spec: filev1.FileTypeSpec{
			Description: "Orders data",
		},
	}
}

func newFileExposure(name string, fileTypeObj *filev1.FileType, zoneConfig *filev1.ZoneServiceConfig) *filev1.FileExposure {
	return &filev1.FileExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "team-a",
			UID:               ktypes.UID("fileexposure-" + name),
			CreationTimestamp: metav1.Now(),
			Labels:            handlerutil.ChildLabels(*ctypes.ObjectRefFromObject(fileTypeObj)),
		},
		Spec: filev1.FileExposureSpec{
			FileType:   fileTypeObj.Name,
			SFTP:       &filev1.FileSFTP{},
			Visibility: filev1.VisibilityEnterprise,
			Approval: filev1.Approval{
				Strategy: filev1.ApprovalStrategySimple,
			},
			Zone: ctypes.ObjectRefFromObject(zoneConfig),
		},
	}
}

func activateFileExposure(fileTypeObj *filev1.FileType, exposure *filev1.FileExposure) {
	fileTypeObj.Status.FileExposureRef = ctypes.ObjectRefFromObject(exposure)
	exposure.Status.FileTypeRef = ctypes.ObjectRefFromObject(fileTypeObj)
}

func newFileSubscription(name string, fileTypeObj *filev1.FileType) *filev1.FileSubscription {
	return &filev1.FileSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "team-a",
			UID:       ktypes.UID("filesubscription-" + name),
		},
		Spec: filev1.FileSubscriptionSpec{
			FileType: fileTypeObj.Name,
			SFTP:     &filev1.FileSFTP{},
		},
	}
}

func newZoneServiceConfig(name string) *filev1.ZoneServiceConfig {
	return &filev1.ZoneServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "team-a",
			UID:       ktypes.UID("zoneserviceconfig-" + name),
		},
		Spec: filev1.ZoneServiceConfigSpec{
			API: sftpv1.APIEndpoint{
				ClientID:     "client-id",
				ClientSecret: "secret-manager://sftp/" + name,
				Endpoint:     "https://sftp-api." + name + ".example.com",
				Issuer:       "https://issuer." + name + ".example.com/token",
			},
			Service: sftpv1.ServiceEndpoint{
				Host: "sftp." + name + ".svc",
				Port: 3022,
			},
			ServiceExternal: sftpv1.ServiceEndpoint{
				Host: "sftp." + name + ".example.com",
				Port: 3022,
			},
		},
	}
}

func haveReadyCondition() types.GomegaMatcher {
	return WithTransform(func(conditions []metav1.Condition) bool {
		return apiMeta.IsStatusConditionTrue(conditions, condition.ConditionTypeReady)
	}, BeTrue())
}

func haveNotReadyCondition(reason string) types.GomegaMatcher {
	return WithTransform(func(conditions []metav1.Condition) bool {
		ready := apiMeta.FindStatusCondition(conditions, condition.ConditionTypeReady)
		return ready != nil && ready.Status == metav1.ConditionFalse && ready.Reason == reason
	}, BeTrue())
}
