// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Context", func() {

	It("should manage the client in the context", func() {
		ctx := context.Background()

		By("setting the client in the context")
		ctx = WithClient(ctx, NewJanitorClient(NewScopedClient(k8sClient, environment)))

		By("getting the client from the context")
		client, ok := ClientFromContext(ctx)
		Expect(ok).To(BeTrue())
		Expect(client).To(BeAssignableToTypeOf(&janitorClient{}))
	})

	It("should panic if the client is not found in the context", func() {
		ctx := context.Background()

		By("getting the client from the context")
		Expect(func() {
			ClientFromContextOrDie(ctx)
		}).To(Panic())
	})
})
