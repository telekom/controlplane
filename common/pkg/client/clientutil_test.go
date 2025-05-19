// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("Client Util Tests", func() {

	// my faked clientObject which only contains the necessary UID field
	fakeClientObject := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"uid": "12345",
			},
		},
	}

	Context("OwnedBy function", func() {
		It("should return correct listOptions", func() {

			listOptions := OwnedBy(fakeClientObject)
			Expect(listOptions).To(HaveLen(1))
			Expect(listOptions[0].(client.MatchingFields)[".metadata.controller"]).To(Equal("12345"))
		})
	})

	Context("OwnedByLabel function", func() {
		It("should return correct listOptions", func() {

			listOptions := OwnedByLabel(fakeClientObject)
			Expect(listOptions).To(HaveLen(1))
			Expect(listOptions[0].(client.MatchingLabels)["cp.ei.telekom.de/owner.uid"]).To(Equal("12345"))
		})
	})

})
