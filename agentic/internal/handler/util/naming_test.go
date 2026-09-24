// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestAgenticRouteNameBounds(t *testing.T) {
	g := NewWithT(t)
	g.Expect(MakeAgenticRouteName("/orders/v1")).To(Equal("ai-gateway--orders-v1"))
	basePath := "/" + strings.Repeat("a", 253)
	name := MakeAgenticRouteName(basePath)
	g.Expect(validation.IsDNS1123Subdomain(name)).To(BeEmpty())
	g.Expect(name).To(Equal(MakeAgenticRouteName(basePath)))
	g.Expect(name).NotTo(Equal(MakeAgenticRouteName(basePath + "b")))
}
