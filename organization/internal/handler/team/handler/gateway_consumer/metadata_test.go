// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_consumer

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestLongTeamConsumerName(t *testing.T) {
	g := NewWithT(t)
	owner := &organizationv1.Team{Spec: organizationv1.TeamSpec{Group: strings.Repeat("g", 130), Name: strings.Repeat("t", 130)}}
	obj := buildGatewayConsumerObj(owner)
	g.Expect(validation.IsDNS1123Subdomain(obj.Name)).To(BeEmpty())
	g.Expect(makeConsumerName(owner)).To(Equal(owner.Spec.Group + "--" + owner.Spec.Name + "--team-user"))
	g.Expect(validation.IsDNS1123Subdomain(organizationv1.TeamResourceName(owner.Spec.Group, owner.Spec.Name))).To(BeEmpty())
	owner.Spec.Name += "a"
	g.Expect(obj.Name).NotTo(Equal(buildGatewayConsumerObj(owner).Name))
}
