// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package xdsapi_test

import (
	"google.golang.org/protobuf/proto"

	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bundle digest", func() {
	It("is deterministic and excludes the digest field", func() {
		bundle := &xdsapi.Bundle{
			TargetId: "target-a", PublisherGeneration: "42", SchemaVersion: xdsapi.SchemaVersion,
			Sources: []*xdsapi.SourceReference{{Kind: "Gateway", Name: "gateway-a", Generation: 7}},
		}

		first, err := xdsapi.Digest(bundle)
		Expect(err).NotTo(HaveOccurred())
		bundle.Digest = "ignored-value"
		second, err := xdsapi.Digest(bundle)
		Expect(err).NotTo(HaveOccurred())
		Expect(second).To(Equal(first))

		bundle.Sources[0].Generation++
		changed, err := xdsapi.Digest(bundle)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).NotTo(Equal(first))
	})

	It("produces stable deterministic envelope bytes", func() {
		bundle := &xdsapi.Bundle{TargetId: "target-a", SchemaVersion: xdsapi.SchemaVersion}
		Expect(xdsapi.SetDigest(bundle)).To(Succeed())
		first, err := xdsapi.MarshalDeterministic(bundle)
		Expect(err).NotTo(HaveOccurred())
		second, err := xdsapi.MarshalDeterministic(proto.Clone(bundle).(*xdsapi.Bundle))
		Expect(err).NotTo(HaveOccurred())
		Expect(second).To(Equal(first))
	})
})
