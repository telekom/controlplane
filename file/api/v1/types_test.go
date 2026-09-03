// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	v1 "github.com/telekom/controlplane/file/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("MakeFileTypeName", func() {
	DescribeTable("converts file type strings to Kubernetes resource names",
		func(input, expected string) {
			Expect(v1.MakeFileTypeName(input)).To(Equal(expected))
		},
		Entry("file type with dots", "de.telekom.eni.foo.v1", "de-telekom-eni-foo-v1"),
		Entry("already hyphenated", "de-telekom-eni-foo-v1", "de-telekom-eni-foo-v1"),
		Entry("empty string", "", ""),
		Entry("mixed case with dots", "De.Telekom.V1", "de-telekom-v1"),
	)
})

var _ = Describe("FileType", func() {
	It("gets and sets conditions", func() {
		ft := &v1.FileType{
			ObjectMeta: metav1.ObjectMeta{Name: "demo-v1", Namespace: "team-ns"},
			Spec:       v1.FileTypeSpec{Type: "demo-v1", Description: "demo"},
		}
		Expect(ft.GetConditions()).To(BeEmpty())
		changed := ft.SetCondition(metav1.Condition{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "Provisioned",
		})
		Expect(changed).To(BeTrue())
		Expect(ft.GetConditions()).To(HaveLen(1))
		Expect(ft.GetConditions()[0].Type).To(Equal("Ready"))
	})

	It("exposes list items via GetItems", func() {
		list := &v1.FileTypeList{Items: []v1.FileType{
			{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
		}}
		items := list.GetItems()
		Expect(items).To(HaveLen(2))
		Expect(items[0].GetName()).To(Equal("a"))
		Expect(items[1].GetName()).To(Equal("b"))
	})
})

var _ = Describe("FileExposure", func() {
	It("deep-copies spec and status without aliasing", func() {
		orig := &v1.FileExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "foo-v1--provider", Namespace: "team-ns"},
			Spec: v1.FileExposureSpec{
				Approval:   v1.Approval{Strategy: v1.ApprovalStrategySimple},
				Visibility: v1.VisibilityEnterprise,
				FileType:   "foo-v1",
				Sftp: v1.SftpExposure{
					PublicKeys: []v1.PublicKey{
						{Label: "provider-key", Key: "ssh-ed25519 AAAA"},
					},
				},
			},
			Status: v1.FileExposureStatus{
				Active:        true,
				Subscriptions: []ctypes.ObjectRef{{Name: "sub", Namespace: "team-ns"}},
			},
		}

		clone := orig.DeepCopy()
		Expect(clone).To(Equal(orig))

		// Mutating the clone must not affect the original.
		clone.Spec.Sftp.PublicKeys[0].Key = "changed"
		clone.Status.Subscriptions[0].Name = "other"
		Expect(orig.Spec.Sftp.PublicKeys[0].Key).To(Equal("ssh-ed25519 AAAA"))
		Expect(orig.Status.Subscriptions[0].Name).To(Equal("sub"))
	})

	It("gets and sets conditions", func() {
		exp := &v1.FileExposure{}
		Expect(exp.SetCondition(metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Pending"})).To(BeTrue())
		Expect(exp.GetConditions()).To(HaveLen(1))
	})
})

var _ = Describe("FileSubscription", func() {
	It("deep-copies public keys without aliasing", func() {
		orig := &v1.FileSubscription{
			Spec: v1.FileSubscriptionSpec{
				FileType: "foo-v1",
				Sftp: v1.SftpSubscription{
					PublicKeys: []v1.PublicKey{{Label: "consumer-key", Key: "ssh-ed25519 BBBB"}},
				},
			},
		}
		clone := orig.DeepCopy()
		Expect(clone).To(Equal(orig))
		clone.Spec.Sftp.PublicKeys[0].Label = "changed"
		Expect(orig.Spec.Sftp.PublicKeys[0].Label).To(Equal("consumer-key"))
	})
})
