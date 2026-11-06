// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TLSConfig", func() {
	It("deep copies", func() {
		in := &TLSConfig{ConfigMapRef: ConfigMapKeyRef{Name: "custom-ca", Key: "ca.pem"}}
		out := in.DeepCopy()
		Expect(out).To(Equal(in))
		out.ConfigMapRef.Name = "other"
		Expect(in.ConfigMapRef.Name).To(Equal("custom-ca"))

		ref := in.ConfigMapRef.DeepCopy()
		ref.Key = "x"
		Expect(in.ConfigMapRef.Key).To(Equal("ca.pem"))
	})

	It("handles nil", func() {
		var in *TLSConfig
		Expect(in.DeepCopy()).To(BeNil())
		var ref *ConfigMapKeyRef
		Expect(ref.DeepCopy()).To(BeNil())
	})
})
