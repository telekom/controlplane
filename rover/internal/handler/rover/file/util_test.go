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
		Entry("dotted file type is normalized", "de.telekom.foo.v1", "consumer", "de-telekom-foo-v1--consumer"),
		Entry("mixed case is lowercased", "De.Telekom.V1", "app", "de-telekom-v1--app"),
		Entry("owner name is normalized", "de.telekom.foo.v1", "My_App", "de-telekom-foo-v1--my-app"),
	)
})

var _ = Describe("mapPublicKeys", func() {
	It("yields nil for nil input", func() {
		Expect(mapPublicKeys(nil)).To(BeNil())
	})

	It("yields nil for an empty slice", func() {
		Expect(mapPublicKeys([]roverv1.PublicKey{})).To(BeNil())
	})

	It("maps key preserving order (label is not carried by the file domain)", func() {
		in := []roverv1.PublicKey{
			{Label: "provider-key", Key: "ssh-ed25519 AAAA"},
			{Label: "consumer-key", Key: "ssh-ed25519 BBBB"},
		}
		got := mapPublicKeys(in)
		Expect(got).To(HaveLen(2))
		Expect(got[0].Key).To(Equal("ssh-ed25519 AAAA"))
		Expect(got[1].Key).To(Equal("ssh-ed25519 BBBB"))
	})
})
