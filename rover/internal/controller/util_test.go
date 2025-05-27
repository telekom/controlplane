// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/telekom/controlplane/common/pkg/config"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func createRover(name string, ns string, env string, spec roverv1.RoverSpec) *roverv1.Rover {
	rover := &roverv1.Rover{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				config.EnvironmentLabelKey: env,
			},
		},
		Spec: spec,
	}

	return rover
}

func newTeam(name, group, namespace, env string) *organizationv1.Team {
	return &organizationv1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      group + "--" + name,
			Namespace: namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: env,
			},
		},
		Spec: organizationv1.TeamSpec{
			Name:    name,
			Group:   group,
			Email:   "team@mail.de",
			Members: []organizationv1.Member{{Email: "member@mail.de", Name: "member"}},
		},
		Status: organizationv1.TeamStatus{},
	}
}
