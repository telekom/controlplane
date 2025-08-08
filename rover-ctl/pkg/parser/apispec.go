// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// ParseApiSpecification is needed because we need to extract the name
// using the basePath defined in the specification.
// For OpenAPI 3.x, we use the servers section to determine the name.
// For Swagger 2.0, we use the basePath.
// The name is sanitized to be a valid Kubernetes resource name.
func ParseApiSpecification(obj types.Object) error {
	b, err := json.Marshal(obj.GetContent())
	if err != nil {
		return errors.Wrap(err, "failed to marshal OpenAPI spec content")
	}
	config := datamodel.NewDocumentConfiguration()
	document, err := libopenapi.NewDocumentWithConfiguration(b, config)
	if err != nil {
		return errors.Wrap(err, "failed to parse OpenAPI document")
	}

	version := document.GetVersion()
	if strings.HasPrefix(version, "2.") {
		name, err := GetNameFromSwaggerSpec(document)
		if err != nil {
			return errors.Wrap(err, "failed to get name from Swagger spec")
		}
		obj.SetName(name)
	}

	if strings.HasPrefix(version, "3.") {
		name, err := GetNameFromOpenapiSpec(document)
		if err != nil {
			return errors.Wrap(err, "failed to get name from OpenAPI spec")
		}
		obj.SetName(name)
	}

	return nil
}

func GetNameFromOpenapiSpec(document libopenapi.Document) (string, error) {
	model, errs := document.BuildV3Model()
	if errs != nil {
		for _, err := range errs {
			log.L().Error(err, "failed to build OpenAPI v3 model")
		}
		return "", errors.New("failed to build OpenAPI v3 model")
	}
	if len(model.Model.Servers) == 0 {
		return "", errors.New("there are no servers in the spec")
	}
	path, err := GetPathFromURL(model.Model.Servers[0].URL)
	if err != nil {
		return "", errors.Wrap(err, "failed to make name from url")
	}
	return SanitizeName(path), nil
}

func GetNameFromSwaggerSpec(document libopenapi.Document) (string, error) {
	model, errs := document.BuildV2Model()
	if errs != nil {
		for _, err := range errs {
			log.L().Error(err, "failed to build Swagger v2 model")
		}
		return "", errors.New("failed to build Swagger v2 model")
	}
	return SanitizeName(model.Model.BasePath), nil
}

func SanitizeName(basePath string) string {
	return strings.Trim(strings.ReplaceAll(basePath, "/", "-"), "-")
}

func GetPathFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse url")
	}

	return u.Path, nil
}
