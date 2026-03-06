// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// MapResponse maps an EventSpecification CRD and its optional file content
// to the API response type.
func MapResponse(in *roverv1.EventSpecification, specContent map[string]any) (res api.EventSpecificationResponse, err error) {
	if in == nil {
		return res, errors.New("input event specification crd is nil")
	}

	res = api.EventSpecificationResponse{
		Type:        in.Spec.Type,
		Version:     in.Spec.Version,
		Description: in.Spec.Description,
		Id:          mapper.MakeResourceId(in),
	}

	if specContent != nil {
		res.Specification = specContent
	}

	res.Status = status.MapStatus(in.Status.Conditions)

	return
}
