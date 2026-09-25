// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
)

// MapRequest maps an API FileSpecification request to the CRD representation.
// It sets the TypeMeta, name (derived from the file type), namespace, and labels.
func MapRequest(req api.FileSpecification, id mapper.ResourceIdInfo) (*roverv1.FileSpecification, error) {
	fileSpec := &roverv1.FileSpecification{}

	fileSpec.TypeMeta = metav1.TypeMeta{
		Kind:       "FileSpecification",
		APIVersion: "rover.cp.ei.telekom.de/v1",
	}

	fileSpec.Spec.Type = req.Type
	fileSpec.Spec.Version = req.Version
	fileSpec.Spec.Description = req.Description

	// Derive the resource name from the file type (dots → hyphens)
	fileSpec.Name = roverv1.MakeFileTypeName(req.Type)

	if fileSpec.Name != id.Name {
		return nil, errors.Errorf("file specification name %q does not match expected name %q", fileSpec.Name, id.Name)
	}

	fileSpec.Namespace = id.Environment + "--" + id.Namespace
	fileSpec.Labels = map[string]string{
		config.EnvironmentLabelKey: id.Environment,
	}

	return fileSpec, nil
}
