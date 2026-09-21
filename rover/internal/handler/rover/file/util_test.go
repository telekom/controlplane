// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package file

import (
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MakeName", func() {
	DescribeTable("normalizes file type and owner into a resource name",
		func(fileType, owner, want string) {
			Expect(MakeName(fileType, owner)).To(Equal(want))
		},
		Entry("hyphenated file type", "de-telekom-eni-foo-v1", "provider", "de-telekom-eni-foo-v1--provider"),
		Entry("dotted file type is normalized", "de-telekom-foo-v1", "consumer", "de-telekom-foo-v1--consumer"),
	)
})

var _ = Describe("mapPublicKeys", func() {
	It("yields nil for nil input", func() {
		Expect(mapSFTP(nil)).To(BeNil())
	})

	It("yields nil for an empty slice", func() {
		Expect(mapSFTP(&roverv1.FileSFTP{PublicKeys: []roverv1.SSHPublicKeySpec{}})).To(BeNil())
	})

	It("maps label and key preserving order", func() {
		in := &roverv1.FileSFTP{
			PublicKeys: []roverv1.SSHPublicKeySpec{
				{Key: "ssh-ed25519 AAAA"},
				{Key: "ssh-ed25519 BBBB"},
			},
		}
		got := mapSFTP(in)
		Expect(got.PublicKeys).To(HaveLen(2))
		Expect(got.PublicKeys[0].Key).To(Equal("ssh-ed25519 AAAA"))
		Expect(got.PublicKeys[1].Key).To(Equal("ssh-ed25519 BBBB"))
	})
})
