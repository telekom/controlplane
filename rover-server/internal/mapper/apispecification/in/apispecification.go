// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/config"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/mapper"
)

var (
	parseErr = "failed to parse specification"
)

func MapRequest(spec *roverv1.ApiSpecificationSpec, fileAPIResp *filesapi.FileUploadResponse, id mapper.ResourceIdInfo) (res *roverv1.ApiSpecification, err error) {
	if fileAPIResp == nil {
		return nil, errors.New("response from file manager is nil")
	}

	if spec == nil {
		return nil, errors.New("input api specification is nil")

	}

	spec.Hash = fileAPIResp.FileHash
	spec.Specification = fileAPIResp.FileId

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
		Spec: *spec,
	}
	return
}
