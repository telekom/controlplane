// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identity_client

import (
	"testing"

	. "github.com/onsi/gomega"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildIdentityClientObj(t *testing.T) {
	RegisterTestingT(t)
	team := &organizationv1.Team{
		Spec: organizationv1.TeamSpec{
			Name:  "team",
			Group: "group",
		},
		Status: organizationv1.TeamStatus{
			Namespace: "env--group--team",
		},
	}

	expected := &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "group--team--team-user",
			Namespace: "env--group--team",
		},
	}
	got := buildIdentityClientObj(team)
	Expect(got).To(Equal(expected))
}
