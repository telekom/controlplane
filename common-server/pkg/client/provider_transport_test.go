// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NewProviderTransport idle timeout", func() {
	It("defaults to a finite idle timeout without a request deadline", func() {
		tr, err := NewProviderTransport(&http.Transport{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(tr.IdleConnTimeout).To(Equal(DefaultProviderIdleConnTimeout))
		Expect(tr.ResponseHeaderTimeout).To(BeZero())
	})

	It("keeps an explicit idle timeout", func() {
		tr, err := NewProviderTransport(&http.Transport{IdleConnTimeout: time.Second}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(tr.IdleConnTimeout).To(Equal(time.Second))
	})
})
