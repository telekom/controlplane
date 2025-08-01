// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"gopkg.in/yaml.v3"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
)

func MapResponse(in *roverv1.ApiSpecification) (res api.ApiSpecificationResponse, err error) {
	if in == nil {
		return res, errors.New("input api specification is nil")
	}
	m := map[string]any{}
	err = yaml.Unmarshal([]byte(in.Spec.Specification), &m)
	if err != nil {
		return
	}

	res = api.ApiSpecificationResponse{
		Category:      in.Status.Category,
		Specification: m,
		Id:            mapper.MakeResourceId(in),
	}
	res.Status = status.MapStatus(in.Status.Conditions)

	return
}
