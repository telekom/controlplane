// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ProviderTLS", func() {
	It("treats nil tls and nil refs as system trust", func() {
		var t *ProviderTLS
		Expect(t.Ref()).To(BeNil())
		Expect(t.Validate()).To(Succeed())
		Expect((&ProviderTLS{}).Validate()).To(Succeed())
	})

	DescribeTable("validates name and key syntax",
		func(name, key string, ok bool) {
			err := (&ProviderTLS{CABundleConfigMapRef: &CABundleConfigMapRef{Name: name, Key: key}}).Validate()
			if ok {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
			}
		},
		Entry("valid", "private-provider-ca", "ca.crt", true),
		Entry("empty name", "", "ca.crt", false),
		Entry("uppercase name", "Bad", "ca.crt", false),
		Entry("empty key", "ca", "", false),
		Entry("invalid key", "ca", "a/b", false),
	)

	It("requires the exact label value", func() {
		Expect(HasProviderCALabel(map[string]string{ProviderCALabelKey: "true"})).To(BeTrue())
		Expect(HasProviderCALabel(map[string]string{ProviderCALabelKey: "True"})).To(BeFalse())
		Expect(HasProviderCALabel(nil)).To(BeFalse())
	})

	It("deep copies the reference", func() {
		in := &ProviderTLS{CABundleConfigMapRef: &CABundleConfigMapRef{Name: "a", Key: "b"}}
		out := in.DeepCopy()
		out.CABundleConfigMapRef.Name = "c"
		Expect(in.CABundleConfigMapRef.Name).To(Equal("a"))
		var nilTLS *ProviderTLS
		Expect(nilTLS.DeepCopy()).To(BeNil())
	})
})
