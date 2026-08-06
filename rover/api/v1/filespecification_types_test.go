// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("FileSpecification Types", func() {
	Context("MakeFileSpecificationName", func() {
		DescribeTable("normalizes the FileSpecification name",
			func(name, expected string) {
				fileSpec := &v1.FileSpecification{
					ObjectMeta: metav1.ObjectMeta{Name: name},
				}
				Expect(v1.MakeFileSpecificationName(fileSpec)).To(Equal(expected))
			},
			Entry("dotted name is hyphenated", "de.telekom.foo.v1", "de-telekom-foo-v1"),
			Entry("mixed case is lowercased", "De.Telekom.V1", "de-telekom-v1"),
			Entry("already hyphenated name is unchanged", "demo-sftp-spec-v1", "demo-sftp-spec-v1"),
		)
	})

	Context("FileStorageType", func() {
		It("should stringify the sftp storage type", func() {
			Expect(v1.FileStorageTypeSFTP.String()).To(Equal("sftp"))
		})
	})

	Context("FileSpecification conditions", func() {
		It("should set and get conditions", func() {
			fileSpec := &v1.FileSpecification{}
			Expect(fileSpec.GetConditions()).To(BeEmpty())

			changed := fileSpec.SetCondition(metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "Provisioned",
				Message: "FileType is ready",
			})
			Expect(changed).To(BeTrue())

			conditions := fileSpec.GetConditions()
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal("Ready"))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))

			// Setting the same condition again reports no change.
			changed = fileSpec.SetCondition(metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "Provisioned",
				Message: "FileType is ready",
			})
			Expect(changed).To(BeFalse())
		})
	})

	Context("FileSpecificationList", func() {
		It("should return its items as types.Object", func() {
			list := &v1.FileSpecificationList{
				Items: []v1.FileSpecification{
					{ObjectMeta: metav1.ObjectMeta{Name: "spec-a"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "spec-b"}},
				},
			}

			items := list.GetItems()
			Expect(items).To(HaveLen(2))
			Expect(items[0].GetName()).To(Equal("spec-a"))
			Expect(items[1].GetName()).To(Equal("spec-b"))
		})

		It("should return an empty slice for an empty list", func() {
			list := &v1.FileSpecificationList{}
			Expect(list.GetItems()).To(BeEmpty())
		})
	})

	Context("SSHKeyType", func() {
		It("should stringify the supported key types", func() {
			Expect(v1.SSHKeyTypeRSA.String()).To(Equal("ssh-rsa"))
			Expect(v1.SSHKeyTypeECDSANistP521.String()).To(Equal("ecdsa-sha2-nistp521"))
			Expect(v1.SSHKeyTypeED25519.String()).To(Equal("ssh-ed25519"))
		})

		DescribeTable("reports validity of a key type",
			func(keyType v1.SSHKeyType, valid bool) {
				Expect(keyType.IsValid()).To(Equal(valid))
			},
			Entry("ssh-rsa is valid", v1.SSHKeyTypeRSA, true),
			Entry("ecdsa-sha2-nistp521 is valid", v1.SSHKeyTypeECDSANistP521, true),
			Entry("ssh-ed25519 is valid", v1.SSHKeyTypeED25519, true),
			Entry("unsupported ecdsa-sha2-nistp256 is invalid", v1.SSHKeyType("ecdsa-sha2-nistp256"), false),
			Entry("empty is invalid", v1.SSHKeyType(""), false),
		)
	})
})
