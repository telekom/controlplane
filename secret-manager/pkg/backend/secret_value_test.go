// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
)

var _ = Describe("Secret Value", func() {

	Context("Default Secret Value Implementation", func() {

		It("should return the correct String value", func() {
			secretVal := backend.String("test")
			Expect(secretVal.Value()).To(Equal("test"))
			Expect(secretVal.IsEmpty()).To(BeFalse())
			Expect(secretVal.AllowChange()).To(BeTrue())
			Expect(secretVal.EqualString("test")).To(BeTrue())
		})

		It("should return the correct JSON value", func() {
			secretVal, err := backend.JSON(map[string]string{"key": "value"})
			Expect(err).ToNot(HaveOccurred())
			Expect(secretVal.Value()).To(Equal("{\"key\":\"value\"}"))
			Expect(secretVal.IsEmpty()).To(BeFalse())
			Expect(secretVal.AllowChange()).To(BeTrue())
		})

		It("should return an empty value and an error on invalid JSON", func() {
			secretVal, err := backend.JSON(make(chan int))
			Expect(err).To(HaveOccurred())
			Expect(secretVal.Value()).To(Equal(""))
			Expect(secretVal.IsEmpty()).To(BeTrue())
			Expect(secretVal.AllowChange()).To(BeFalse())
		})

		It("should return the correct initial value", func() {
			secretVal := backend.InitialString("initial")
			Expect(secretVal.Value()).To(Equal("initial"))
			Expect(secretVal.IsEmpty()).To(BeFalse())
			Expect(secretVal.AllowChange()).To(BeFalse())
			Expect(secretVal.EqualString("initial")).To(BeTrue())
		})
	})
})
