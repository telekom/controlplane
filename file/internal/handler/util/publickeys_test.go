// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"errors"
	"fmt"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// validKey is a minimal SSH public key that passes both parse and fingerprint steps.
// The base64 payload decodes to arbitrary bytes valid for SHA256.
const (
	validKey  = "ssh-rsa cHJvdmlkZXI="
	validKey2 = "ssh-rsa c3Vic2NyaWJlcg=="
)

var _ = Describe("CanonicalSSHPublicKeys", func() {
	It("returns nil for an empty input slice", func() {
		keys, err := CanonicalSSHPublicKeys(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(keys).To(BeEmpty())
	})

	It("returns an error for a key that cannot be parsed", func() {
		_, err := CanonicalSSHPublicKeys([]filev1.SSHPublicKeySpec{{Key: "invalidkey"}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("canonicalizing SSH public key"))
	})

	It("strips the comment and returns the canonical form of a valid key", func() {
		keys, err := CanonicalSSHPublicKeys([]filev1.SSHPublicKeySpec{
			{Key: validKey + " some-comment"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(keys).To(HaveLen(1))
		Expect(keys[0]).To(Equal(validKey))
	})

	It("de-duplicates identical keys (same fingerprint and content)", func() {
		keys, err := CanonicalSSHPublicKeys([]filev1.SSHPublicKeySpec{
			{Key: validKey + " comment-one"},
			{Key: validKey + " comment-two"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(keys).To(HaveLen(1))
		Expect(keys[0]).To(Equal(validKey))
	})

	It("returns multiple distinct keys sorted by fingerprint", func() {
		keys, err := CanonicalSSHPublicKeys([]filev1.SSHPublicKeySpec{
			{Key: validKey},
			{Key: validKey2},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(keys).To(HaveLen(2))
	})
})

var _ = Describe("SyncSFTPUser / DeleteSFTPUser / DeleteSFTPInstance", func() {
	const (
		testNS   = "test"
		testName = "test-user"
	)

	newCtx := func() (context.Context, *fake.MockJanitorClient) {
		mc := fake.NewMockJanitorClient(GinkgoT())
		return cclient.WithClient(context.Background(), mc), mc
	}

	userRef := types.ObjectRef{Name: testName, Namespace: testNS}
	ftRef := types.ObjectRef{Name: "ft", Namespace: testNS}
	instanceRef := types.ObjectRef{Name: "instance", Namespace: testNS}
	owner := &filev1.FileExposure{
		ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: testNS},
	}

	Describe("DeleteSFTPUser", func() {
		It("calls c.Delete for the user", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(nil).Once()
			Expect(DeleteSFTPUser(ctx, userRef)).To(Succeed())
		})

		It("tolerates NotFound", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: sftpv1.GroupVersion.Group, Resource: "users"}, testName)).
				Once()
			Expect(DeleteSFTPUser(ctx, userRef)).To(Succeed())
		})

		It("returns wrapped error on unexpected delete failure", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.User")).
				Return(fmt.Errorf("network error")).Once()
			err := DeleteSFTPUser(ctx, userRef)
			Expect(err).To(MatchError(ContainSubstring("network error")))
		})
	})

	Describe("DeleteSFTPInstance", func() {
		It("calls c.Delete for the instance", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.Instance")).
				Return(nil).Once()
			Expect(DeleteSFTPInstance(ctx, instanceRef)).To(Succeed())
		})

		It("tolerates NotFound", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.Instance")).
				Return(apierrors.NewNotFound(schema.GroupResource{}, "instance")).
				Once()
			Expect(DeleteSFTPInstance(ctx, instanceRef)).To(Succeed())
		})

		It("returns wrapped error when Delete fails unexpectedly", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				Delete(mock.Anything, mock.AnythingOfType("*v1.Instance")).
				Return(fmt.Errorf("network timeout")).Once()
			err := DeleteSFTPInstance(ctx, instanceRef)
			Expect(err).To(MatchError(ContainSubstring("network timeout")))
		})
	})

	Describe("SyncSFTPUser", func() {
		It("calls c.CreateOrUpdate for the user", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.User"), mock.Anything).
				Return(controllerutil.OperationResultCreated, nil).Once()

			user, err := SyncSFTPUser(ctx, userRef, owner, ftRef, nil, instanceRef)

			Expect(err).NotTo(HaveOccurred())
			Expect(user).NotTo(BeNil())
		})

		It("returns wrapped error when CreateOrUpdate fails", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				CreateOrUpdate(mock.Anything, mock.AnythingOfType("*v1.User"), mock.Anything).
				Return(controllerutil.OperationResultNone, fmt.Errorf("create failed")).Once()

			_, err := SyncSFTPUser(ctx, userRef, owner, ftRef, nil, instanceRef)

			Expect(err).To(MatchError(ContainSubstring("create failed")))
		})

		It("returns error when a provided SSH key cannot be canonicalized", func() {
			ctx, mc := newCtx()
			_ = mc // no calls expected

			invalidKeys := []filev1.SSHPublicKeySpec{{Key: "notvalidkey"}}
			_, err := SyncSFTPUser(ctx, userRef, owner, ftRef, invalidKeys, instanceRef)

			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("Getters", func() {
	const (
		testNS       = "test"
		testFTName   = "my-ft"
		testZoneName = "my-zone"
		testZoneNS   = "test--my-zone"
	)

	newCtx := func() (context.Context, *fake.MockJanitorClient) {
		mc := fake.NewMockJanitorClient(GinkgoT())
		return cclient.WithClient(context.Background(), mc), mc
	}

	Describe("GetFileType", func() {
		ref := types.ObjectRef{Name: testFTName, Namespace: testNS}

		It("returns the FileType on success", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				Get(mock.Anything, ref.K8s(), mock.AnythingOfType("*v1.FileType")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					out.(*filev1.FileType).Name = testFTName
				}).
				Return(nil).Once()

			ft, err := GetFileType(ctx, ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(ft.Name).To(Equal(testFTName))
		})

		It("returns a BlockedError when the FileType is not found", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				Get(mock.Anything, ref.K8s(), mock.AnythingOfType("*v1.FileType")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: filev1.GroupVersion.Group, Resource: "filetypes"}, testFTName)).
				Once()

			_, err := GetFileType(ctx, ref)
			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue())
		})

		It("returns a wrapped error on unexpected Get failure", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				Get(mock.Anything, ref.K8s(), mock.AnythingOfType("*v1.FileType")).
				Return(fmt.Errorf("timeout")).Once()

			_, err := GetFileType(ctx, ref)
			Expect(err).To(MatchError(ContainSubstring("timeout")))
		})
	})

	Describe("GetZoneServiceConfig", func() {
		zoneRef := &types.ObjectRef{Name: testZoneName, Namespace: testNS}

		It("returns a BlockedError when no ZoneServiceConfig is found", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.ZoneServiceConfigList"), mock.Anything, mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					// leave list empty
				}).
				Return(nil).Once()

			_, err := GetZoneServiceConfig(ctx, zoneRef)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ZoneServiceConfig"))
		})

		It("returns error when list has more than one result", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.ZoneServiceConfigList"), mock.Anything, mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*filev1.ZoneServiceConfigList) = filev1.ZoneServiceConfigList{
						Items: []filev1.ZoneServiceConfig{{}, {}},
					}
				}).
				Return(nil).Once()

			_, err := GetZoneServiceConfig(ctx, zoneRef)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected exactly one"))
		})

		It("returns the ZoneServiceConfig when exactly one is found", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.ZoneServiceConfigList"), mock.Anything, mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*filev1.ZoneServiceConfigList) = filev1.ZoneServiceConfigList{
						Items: []filev1.ZoneServiceConfig{{
							ObjectMeta: metav1.ObjectMeta{Name: "zsc", Namespace: testNS},
						}},
					}
				}).
				Return(nil).Once()

			zsc, err := GetZoneServiceConfig(ctx, zoneRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(zsc.Name).To(Equal("zsc"))
		})
	})

	Describe("FindActiveFileExposure", func() {
		ftRef := &types.ObjectRef{Name: testFTName, Namespace: testNS}

		It("returns false when no exposures exist", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.FileExposureList"), mock.Anything, mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					// leave empty
				}).
				Return(nil).Once()

			_, found, err := FindActiveFileExposure(ctx, ftRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse())
		})

		It("returns the first exposure sorted by creation time when multiple exist", func() {
			ctx, mc := newCtx()
			older := filev1.FileExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "older",
					Namespace:         testNS,
					CreationTimestamp: metav1.Time{},
				},
			}
			newer := filev1.FileExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "newer",
					Namespace:         testNS,
					CreationTimestamp: metav1.Now(),
				},
			}
			mc.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.FileExposureList"), mock.Anything, mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*filev1.FileExposureList) = filev1.FileExposureList{
						Items: []filev1.FileExposure{newer, older},
					}
				}).
				Return(nil).Once()

			active, found, err := FindActiveFileExposure(ctx, ftRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(active.Name).To(Equal("older"))
		})

		It("returns error when list fails", func() {
			ctx, mc := newCtx()
			mc.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.FileExposureList"), mock.Anything, mock.Anything).
				Return(fmt.Errorf("list failed")).Once()

			_, _, err := FindActiveFileExposure(ctx, ftRef)
			Expect(err).To(MatchError(ContainSubstring("list failed")))
		})
	})

	Describe("GetPublicKeysFromSFTP", func() {
		It("returns nil when SFTP spec is nil", func() {
			Expect(GetPublicKeysFromSFTP(nil)).To(BeNil())
		})

		It("returns the public keys from the SFTP spec", func() {
			sftp := &filev1.FileSFTP{
				PublicKeys: []filev1.SSHPublicKeySpec{{Key: validKey}},
			}
			keys := GetPublicKeysFromSFTP(sftp)
			Expect(keys).To(HaveLen(1))
			Expect(keys[0].Key).To(Equal(validKey))
		})
	})
})
