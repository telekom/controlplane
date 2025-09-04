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

func MapResponse(in *roverv1.ApiSpecification, inFile map[string]any) (res api.ApiSpecificationResponse, err error) {
	if in == nil {
		return res, errors.New("input api specification crd is nil")
	}

	if inFile == nil {
		return res, errors.New("input api specification is nil")
	}

	res = api.ApiSpecificationResponse{
		Category:      in.Spec.Category,
		Id:            mapper.MakeResourceId(in),
		Name:          in.Spec.ApiName,
		Specification: inFile,
		VendorApi:     in.Spec.XVendor,
	}
	res.Status = status.MapStatus(in.Status.Conditions)

	return
}
