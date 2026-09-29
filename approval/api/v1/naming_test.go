// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestApprovalNameBounds(t *testing.T) {
	g := NewWithT(t)
	g.Expect(ApprovalName("ApiSubscription", "orders")).To(Equal("apisubscription--orders"))
	owner := &Approval{ObjectMeta: metav1.ObjectMeta{Name: strings.Repeat("a", 253)}}
	name := ApprovalName("ApiSubscription", owner.Name)
	request := ApprovalRequestName(owner, "first")
	g.Expect(validation.IsDNS1123Subdomain(name)).To(BeEmpty())
	g.Expect(validation.IsDNS1123Subdomain(request)).To(BeEmpty())
	g.Expect(request).To(Equal(ApprovalRequestName(owner, "first")))
	g.Expect(request).NotTo(Equal(ApprovalRequestName(owner, "second")))
	g.Expect(name).NotTo(Equal(ApprovalName("EventSubscription", owner.Name)))
}
