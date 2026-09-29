// Copyright 2026 Deutsche Telekom IT GmbH
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

// MapResponse maps a FileSpecification CRD to the API response type.
func MapResponse(in *roverv1.FileSpecification) (res api.FileSpecificationResponse, err error) {
	if in == nil {
		return res, errors.New("input file specification crd is nil")
	}

	res = api.FileSpecificationResponse{
		Type:        in.Spec.Type,
		Version:     in.Spec.Version,
		Description: in.Spec.Description,
		Id:          mapper.MakeResourceId(in),
		Status:      status.MapStatus(in.GetConditions(), in.GetGeneration()),
	}

	return res, nil
}
