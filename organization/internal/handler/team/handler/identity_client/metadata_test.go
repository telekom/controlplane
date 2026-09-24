// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identity_client

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"

	organizationv1 "github.com/telekom/controlplane/organization/api/v1"

	. "github.com/onsi/gomega"
)

func TestLongTeamClientName(t *testing.T) {
	g := NewWithT(t)
	owner := &organizationv1.Team{Spec: organizationv1.TeamSpec{Group: strings.Repeat("g", 130), Name: strings.Repeat("t", 130)}}
	obj := buildIdentityClientObj(owner)
	g.Expect(validation.IsDNS1123Subdomain(obj.Name)).To(BeEmpty())
	g.Expect(len(MakeClientId(owner))).To(BeNumerically(">", 253))
	owner.Spec.Name += "a"
	g.Expect(obj.Name).NotTo(Equal(buildIdentityClientObj(owner).Name))
}
