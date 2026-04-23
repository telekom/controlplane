// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var parseErr = "failed to parse specification"

func MapRequest(apiSpec *roverv1.ApiSpecification, fileAPIResp *filesapi.FileUploadResponse, id mapper.ResourceIdInfo) (err error) {
	if fileAPIResp == nil {
		return errors.New("response from file manager is nil")
	}

	if apiSpec == nil {
		return errors.New("input api specification is nil")
	}

	apiSpec.TypeMeta = metav1.TypeMeta{
		Kind:       "ApiSpecification",
		APIVersion: "rover.cp.ei.telekom.de/v1",
	}

	apiSpec.Spec.Hash = fileAPIResp.FileHash
	apiSpec.Spec.Specification = fileAPIResp.FileId
	apiSpec.Labels = map[string]string{
		config.EnvironmentLabelKey: id.Environment,
	}
	if apiSpec.Name != id.Name {
		return errors.Errorf("api specification name %q does not match expected name %q", apiSpec.Name, id.Name)
	}
	apiSpec.Namespace = id.Environment + "--" + id.Namespace
	return err
}
