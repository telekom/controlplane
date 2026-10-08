// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"encoding/json"
	"strings"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
	"github.com/pkg/errors"
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
		obj.SetProperty("name", name)
	}

	if strings.HasPrefix(version, "3.") {
		name, err := GetNameFromOpenapiSpec(document)
		if err != nil {
			return errors.Wrap(err, "failed to get name from OpenAPI spec")
		}
		obj.SetProperty("name", name)
	}

	return nil
}

func GetNameFromOpenapiSpec(document libopenapi.Document) (string, error) {
	model, err := document.BuildV3Model()
	if err != nil {
		return "", errors.Wrap(err, "failed to build OpenAPI v3 model")
	}
	if len(model.Model.Servers) == 0 {
		return "", errors.New("there are no servers in the spec")
	}
	path, err := GetPathFromURL(model.Model.Servers[0].URL)
	if err != nil {
		return "", errors.Wrap(err, "failed to make name from url")
	}
	if path == "" {
		return "", errors.New("'servers[0].url' must contain a valid path")
	}
	return SanitizeName(path), nil
}

func GetNameFromSwaggerSpec(document libopenapi.Document) (string, error) {
	model, err := document.BuildV2Model()
	if err != nil {
		return "", errors.Wrap(err, "failed to build Swagger v2 model")
	}
	if model.Model.BasePath == "" {
		return "", errors.New("'.basePath' must contain a valid path")
	}
	return SanitizeName(model.Model.BasePath), nil
}

// SanitizeName derives a lowercase resource name from an API base path.
func SanitizeName(basePath string) string {
	return strings.ToLower(strings.Trim(strings.ReplaceAll(basePath, "/", "-"), "-"))
}

func GetPathFromURL(rawURL string) (string, error) {
	path := rawURL
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	// Skip scheme and authority; they may contain undeclared templates like {host}.
	// A colon before the first slash ends the scheme (RFC 3986 forbids it in a relative path's first segment).
	if i := strings.IndexAny(path, ":/"); i > 0 && path[i] == ':' {
		path = path[i+1:]
	}
	if strings.HasPrefix(path, "//") {
		i := strings.Index(path[2:], "/")
		if i < 0 {
			return "", nil
		}
		path = path[2+i:]
	}
	if strings.ContainsAny(path, "{}") {
		return "", errors.Errorf("server url path %q must not contain variables", path)
	}
	if err := validateURLPath(path); err != nil {
		return "", errors.Wrapf(err, "invalid server url path %q", path)
	}
	return path, nil
}

// validateURLPath checks RFC 3986 path syntax: "/" and pchar
// (unreserved, sub-delims, ":", "@" and well-formed percent-encodings).
func validateURLPath(path string) error {
	for i := 0; i < len(path); i++ {
		c := path[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case strings.IndexByte("/-._~!$&'()*+,;=:@", c) >= 0:
		case c == '%':
			if i+2 >= len(path) || !isHex(path[i+1]) || !isHex(path[i+2]) {
				return errors.Errorf("malformed percent-encoding at position %d", i)
			}
			i += 2
		default:
			return errors.Errorf("invalid character %q at position %d", c, i)
		}
	}
	return nil
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}
