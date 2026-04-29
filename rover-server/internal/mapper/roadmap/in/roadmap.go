// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"regexp"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// versionSuffixRe matches a major version suffix like "-v1", "-v2", "-v10"
var versionSuffixRe = regexp.MustCompile(`-v\d+$`)

// MakeRoadmapName generates the roadmap name from an API basePath.
// The name is the normalized basePath with the major version suffix removed.
// Example: "/eni/my-api/v1" → "eni-my-api"
// Note: the name must NOT contain "--" since that is used as a separator in resource IDs.
func MakeRoadmapName(basePath string) string {
	normalized := labelutil.NormalizeValue(basePath)
	specialName := versionSuffixRe.ReplaceAllString(normalized, "")
	return labelutil.NormalizeNameValue(specialName)
}

// MapRequest maps the input parameters to a Roadmap CRD.
func MapRequest(basePath string, fileAPIResp *filesapi.FileUploadResponse, id mapper.ResourceIdInfo) (*roverv1.Roadmap, error) {
	if fileAPIResp == nil {
		return nil, errors.New("response from file manager is nil")
	}

	ns := id.Environment + "--" + id.Namespace
	apiSpecName := labelutil.NormalizeValue(basePath)
	apiSpecRef := types.TypedObjectRef{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ApiSpecification",
			APIVersion: "rover.cp.ei.telekom.de/v1",
		},
		ObjectRef: types.ObjectRef{
			Name:      apiSpecName,
			Namespace: ns,
		},
	}

	roadmap := &roverv1.Roadmap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Roadmap",
			APIVersion: "rover.cp.ei.telekom.de/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      id.Name,
			Namespace: ns,
			Annotations: map[string]string{
				config.BuildLabelKey("basePath"): basePath,
			},
		},
		Spec: roverv1.RoadmapSpec{
			SpecificationRef: apiSpecRef,
			Contents:         fileAPIResp.FileId,
			Hash:             fileAPIResp.FileHash,
		},
	}

	return roadmap, nil
}
