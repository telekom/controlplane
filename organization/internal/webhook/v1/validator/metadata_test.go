// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateNormalizedTeamName(t *testing.T) {
	g := NewWithT(t)
	group, team := strings.Repeat("g", 130), strings.Repeat("t", 130)
	obj := &organizationv1.Team{
		ObjectMeta: metav1.ObjectMeta{Name: organizationv1.TeamResourceName(group, team)},
		Spec:       organizationv1.TeamSpec{Group: group, Name: team},
	}
	g.Expect(ValidateTeamName(obj)).To(Succeed())
	obj.Spec.Name += "a"
	g.Expect(ValidateTeamName(obj)).NotTo(Succeed())
}
