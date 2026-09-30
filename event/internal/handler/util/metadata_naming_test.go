// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"

	. "github.com/onsi/gomega"
)

func TestEventRouteNameBounds(t *testing.T) {
	g := NewWithT(t)
	input := strings.Repeat("a", 253)
	g.Expect(validation.IsDNS1123Subdomain(makeSSERouteName(input))).To(BeEmpty())
	g.Expect(makeSSERouteName(input)).NotTo(Equal(makeSSERouteName(input + "b")))
}
