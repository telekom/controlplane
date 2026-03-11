// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
)

// MapRequest maps an API EventSpecification request to the CRD representation.
// It sets the TypeMeta, name (derived from the event type), namespace, labels,
// and the file-manager reference for the optional specification payload.
func MapRequest(req api.EventSpecification, specOrFileId string, id mapper.ResourceIdInfo) (*roverv1.EventSpecification, error) {
	eventSpec := &roverv1.EventSpecification{}

	eventSpec.TypeMeta = metav1.TypeMeta{
		Kind:       "EventSpecification",
		APIVersion: "rover.cp.ei.telekom.de/v1",
	}

	eventSpec.Spec.Type = req.Type
	eventSpec.Spec.Version = req.Version
	eventSpec.Spec.Description = req.Description

	// Derive the resource name from the event type (dots → hyphens)
	eventSpec.Name = strings.ToLower(strings.ReplaceAll(req.Type, ".", "-"))

	if eventSpec.Name != id.Name {
		return nil, errors.Errorf("event specification name %q does not match expected name %q", eventSpec.Name, id.Name)
	}

	eventSpec.Namespace = id.Environment + "--" + id.Namespace
	eventSpec.Labels = map[string]string{
		config.EnvironmentLabelKey: id.Environment,
	}

	eventSpec.Spec.Specification = specOrFileId

	return eventSpec, nil
}
