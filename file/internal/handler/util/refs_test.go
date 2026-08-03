// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Refs", func() {
	Describe("SFTPUserRefForFileSubscription", func() {
		It("prefixes name with 'filesubscription-' and normalizes it", func() {
			sub := &filev1.FileSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "my-sub", Namespace: "ns"},
			}
			ref := SFTPUserRefForFileSubscription(sub)
			Expect(ref.Namespace).To(Equal("ns"))
			Expect(ref.Name).To(HavePrefix("filesubscription-"))
		})

		It("normalizes names with uppercase characters", func() {
			sub := &filev1.FileSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "My-Sub", Namespace: "ns"},
			}
			ref := SFTPUserRefForFileSubscription(sub)
			Expect(ref.Name).To(Equal("filesubscription-my-sub"))
		})
	})

	Describe("SFTPUserRefForFileExposure", func() {
		It("prefixes name with 'fileexposure-' and normalizes it", func() {
			exposure := &filev1.FileExposure{
				ObjectMeta: metav1.ObjectMeta{Name: "my-exposure", Namespace: "ns"},
			}
			ref := SFTPUserRefForFileExposure(exposure)
			Expect(ref.Namespace).To(Equal("ns"))
			Expect(ref.Name).To(Equal("fileexposure-my-exposure"))
		})
	})

	Describe("SFTPInstanceRefForFileExposure", func() {
		It("uses the FileType name as the instance name", func() {
			exposure := &filev1.FileExposure{
				ObjectMeta: metav1.ObjectMeta{Name: "my-exposure", Namespace: "ns"},
				Spec:       filev1.FileExposureSpec{FileType: "my-filetype"},
			}
			ref := SFTPInstanceRefForFileExposure(exposure)
			Expect(ref.Name).To(Equal("my-filetype"))
			Expect(ref.Namespace).To(Equal("ns"))
		})
	})

	Describe("FileExposureSourceRef", func() {
		It("returns a TypedObjectRef with FileExposure GVK", func() {
			exposure := &filev1.FileExposure{
				ObjectMeta: metav1.ObjectMeta{Name: "exp", Namespace: "ns"},
			}
			ref := FileExposureSourceRef(exposure)
			Expect(ref.Kind).To(Equal("FileExposure"))
			Expect(ref.APIVersion).To(Equal(filev1.GroupVersion.String()))
			Expect(ref.Name).To(Equal("exp"))
		})
	})

	Describe("FileSubscriptionSourceRef", func() {
		It("returns a TypedObjectRef with FileSubscription GVK", func() {
			sub := &filev1.FileSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "sub", Namespace: "ns"},
			}
			ref := FileSubscriptionSourceRef(sub)
			Expect(ref.Kind).To(Equal("FileSubscription"))
			Expect(ref.APIVersion).To(Equal(filev1.GroupVersion.String()))
		})
	})

	Describe("GetChildResourceRef", func() {
		It("prefixes name with 'sftp-api--'", func() {
			obj := &filev1.ZoneServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "my-zone", Namespace: "ns"},
			}
			ref := GetChildResourceRef(obj)
			Expect(ref.Name).To(Equal("sftp-api--my-zone"))
			Expect(ref.Namespace).To(Equal("ns"))
		})
	})

	Describe("Labels", func() {
		Describe("ChildLabels", func() {
			It("returns all expected label keys", func() {
				ft := types.ObjectRef{Name: "my-ft", Namespace: "ns"}
				labels := ChildLabels(ft)
				Expect(labels).To(HaveKey(config.DomainLabelKey))
				Expect(labels[config.DomainLabelKey]).To(Equal("file"))
				Expect(labels).To(HaveKey(filev1.FileTypeNameLabelKey))
				Expect(labels[filev1.FileTypeNameLabelKey]).To(Equal("my-ft"))
			})
		})

		Describe("FileTypeLabelSelector", func() {
			It("returns a non-nil selector", func() {
				sel := FileTypeLabelSelector("my-ft")
				Expect(sel).NotTo(BeNil())
				Expect(sel.String()).To(ContainSubstring("my-ft"))
			})
		})
	})
})
