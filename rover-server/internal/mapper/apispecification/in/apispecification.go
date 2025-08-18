// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
)

func MapRequest(in *api.ApiSpecificationUpdateRequest, id mapper.ResourceIdInfo) (res *roverv1.ApiSpecification, err error) {
	if in == nil {
		return nil, errors.New("input api specification is nil")
	}
	b, err := yaml.Marshal(in.Specification)
	if err != nil {
		return
	}

	res = &roverv1.ApiSpecification{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ApiSpecification",
			APIVersion: "rover.cp.ei.telekom.de/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      id.Name,
			Namespace: id.Environment + "--" + id.Namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: id.Environment,
			},
		},
		Spec: roverv1.ApiSpecificationSpec{
			Specification: string(b),
		},
	}
	return
}
