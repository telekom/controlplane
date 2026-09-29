// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestSpecificationNameBounds(t *testing.T) {
	for name, build := range map[string]func(string) string{
		"agent": MakeAgentSpecificationName,
		"mcp":   MakeMcpSpecificationName,
		"api": func(value string) string {
			return MakeName(&ApiSpecification{Spec: ApiSpecificationSpec{BasePath: value}})
		},
		"event": func(value string) string {
			return MakeEventSpecificationName(&EventSpecification{Spec: EventSpecificationSpec{Type: value}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			input := strings.Repeat("a", 254)
			g.Expect(validation.IsDNS1123Subdomain(build(input))).To(BeEmpty())
			g.Expect(build(input)).To(Equal(build(input)))
			g.Expect(build(input)).NotTo(Equal(build(input + "b")))
			g.Expect(build("orders-v1")).To(Equal("orders-v1"))
		})
	}
}
