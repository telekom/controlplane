// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphqlinputs

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInputs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GraphQL Inputs Suite")
}

var _ = Describe("GraphQL filters", func() {
	It("marshals the supported team filter", func() {
		name := "team-a"
		group := "hub-a"
		value := TeamWhereInput{Name: &name, HasGroupWith: []GroupWhereInput{{Name: &group}}}

		encoded, err := json.Marshal(value)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).To(Equal(`{"name":"team-a","hasGroupWith":[{"name":"hub-a"}]}`))
	})
})
