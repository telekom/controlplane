// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

const testResourceName = "test-resource"

func createRover(spec *roverv1.RoverSpec) *roverv1.Rover {
	rover := &roverv1.Rover{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testResourceName,
			Namespace: teamNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: *spec,
	}

	return rover
}

func newTeam(name, group string) *organizationv1.Team {
	return &organizationv1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      group + "--" + name,
			Namespace: testEnvironment,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: organizationv1.TeamSpec{
			Name:     name,
			Group:    group,
			Email:    "team@mail.de",
			Category: organizationv1.TeamCategoryCustomer,
			Members:  []organizationv1.Member{{Email: "member@mail.de", Name: "member"}},
		},
		Status: organizationv1.TeamStatus{},
	}
}
