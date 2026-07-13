// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/telekom/controlplane/projector/internal/domain/approval"
)

var _ = Describe("FromProperties", func() {
	It("should return empty scopes when properties are unset", func() {
		props, err := approval.FromProperties(approvalv1.Requester{})
		Expect(err).NotTo(HaveOccurred())
		Expect(props.Scopes).To(BeEmpty())
	})

	It("should join multiple scopes with a comma separator", func() {
		req := approvalv1.Requester{}
		Expect(req.SetProperties(map[string]any{"scopes": []string{"read", "write"}})).To(Succeed())

		props, err := approval.FromProperties(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(props.Scopes).To(Equal("read, write"))
	})

	It("should handle a single scope", func() {
		req := approvalv1.Requester{}
		Expect(req.SetProperties(map[string]any{"scopes": []string{"read"}})).To(Succeed())

		props, err := approval.FromProperties(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(props.Scopes).To(Equal("read"))
	})

	It("should return empty scopes when the scopes key is absent", func() {
		req := approvalv1.Requester{}
		Expect(req.SetProperties(map[string]any{"other": "value"})).To(Succeed())

		props, err := approval.FromProperties(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(props.Scopes).To(BeEmpty())
	})

	It("should return an error for malformed properties JSON", func() {
		req := approvalv1.Requester{
			Properties: runtime.RawExtension{Raw: []byte("{not-json")},
		}

		_, err := approval.FromProperties(req)
		Expect(err).To(HaveOccurred())
	})
})
