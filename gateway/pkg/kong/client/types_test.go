// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Suite")
}

var _ = Describe("Upstream", func() {
	It("should parse a valid URL", func() {
		upstream, err := NewUpstream("http://localhost:8080/api/v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(upstream.GetScheme()).To(Equal("http"))
		Expect(upstream.GetHost()).To(Equal("localhost"))
		Expect(upstream.GetPort()).To(Equal(8080))
		Expect(upstream.GetPath()).To(Equal("/api/v1"))
	})

	It("should return error for invalid URL", func() {
		_, err := NewUpstream("://invalid")
		Expect(err).To(HaveOccurred())
	})

	It("should panic on invalid URL with OrDie", func() {
		Expect(func() {
			NewUpstreamOrDie("://invalid")
		}).To(Panic())
	})

	It("should panic on missing port with OrDie", func() {
		Expect(func() {
			NewUpstreamOrDie("http://localhost/path")
		}).To(Panic())
	})

	It("should create upstream from valid URL with OrDie", func() {
		upstream := NewUpstreamOrDie("http://example.com:9090/test")
		Expect(upstream.GetScheme()).To(Equal("http"))
		Expect(upstream.GetHost()).To(Equal("example.com"))
		Expect(upstream.GetPort()).To(Equal(9090))
		Expect(upstream.GetPath()).To(Equal("/test"))
	})
})
