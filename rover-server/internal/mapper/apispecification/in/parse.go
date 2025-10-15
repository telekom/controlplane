// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"context"
	"net/url"
	"strings"

	"github.com/pb33f/libopenapi"
	v2 "github.com/pb33f/libopenapi/datamodel/high/v2"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"go.yaml.in/yaml/v4"
)

func ParseSpecification(ctx context.Context, spec string) (*roverv1.ApiSpecification, error) {
	apiSpecification := &roverv1.ApiSpecification{
		Spec: roverv1.ApiSpecificationSpec{
			XVendor:      false,
			Oauth2Scopes: []string{},
		},
	}
	log := log.FromContext(ctx)

	rawSpec := []byte(strings.TrimSpace(spec))
	document, err := libopenapi.NewDocument(rawSpec)
	if err != nil {
		return nil, errors.Wrap(err, parseErr)
	}

	version := document.GetVersion()
	if strings.HasPrefix(version, "2.") {
		model, err := document.BuildV2Model()
		if err != nil {
			log.Info("failed to build v2 model", zap.Error(err))
			return nil, problems.BadRequest("invalid format of OpenAPI v2 specification")
		}

		apiSpecification.Spec.BasePath = model.Model.BasePath
		apiSpecification.Spec.Version = model.Model.Info.Version
		setExtensionValues(&apiSpecification.Spec, model.Model.Info.Extensions)

		if model.Model.SecurityDefinitions != nil {
			setSecurityDefinitionsValues(&apiSpecification.Spec, model.Model.SecurityDefinitions.Definitions)
		}

		apiSpecification.ObjectMeta.Name = roverv1.MakeName(apiSpecification)

		return apiSpecification, nil
	}

	if strings.HasPrefix(version, "3.") {
		model, err := document.BuildV3Model()
		if err != nil {
			log.Info("failed to build v3 model", zap.Error(err))
			return nil, problems.BadRequest("invalid format of OpenAPI v3 specification")
		}

		if len(model.Model.Servers) == 0 {
			return nil, problems.ValidationErrors(map[string]string{
				"servers[0]": "no servers defined in the specification",
			})
		}

		path, err := getPathFromURL(model.Model.Servers[0].URL)
		if err != nil {
			return nil, problems.ValidationErrors(map[string]string{
				"servers[0].url": "invalid url format",
			})
		} else if path == "" {
			return nil, problems.ValidationErrors(map[string]string{
				"servers[0].url": "no basepath found in the first server url",
			})
		}

		apiSpecification.Spec.BasePath = path

		apiSpecification.Spec.Version = model.Model.Info.Version

		setExtensionValues(&apiSpecification.Spec, model.Model.Info.Extensions)

		if model.Model.Components != nil {
			setSecuritySchemeValues(&apiSpecification.Spec, model.Model.Components.SecuritySchemes)
		}

		apiSpecification.ObjectMeta.Name = roverv1.MakeName(apiSpecification)
		return apiSpecification, nil
	}

	return nil, problems.BadRequest("only OpenAPI v2 and v3 are supported")
}

func getPathFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse url")
	}

	return u.Path, nil
}

func setExtensionValues(apiSpecificationSpec *roverv1.ApiSpecificationSpec, extensionMap *orderedmap.Map[string, *yaml.Node]) {
	if extensionMap == nil {
		return
	}
	value, ok := extensionMap.Get("x-api-category")
	if ok {
		apiSpecificationSpec.Category = value.Value
	}
	value, ok = extensionMap.Get("x-vendor")
	if ok {
		apiSpecificationSpec.XVendor = value.Value == "true"
	}
}

func setSecurityDefinitionsValues(apiSpecificationSpec *roverv1.ApiSpecificationSpec, Definitions *orderedmap.Map[string, *v2.SecurityScheme]) {
	if Definitions.Len() == 0 {
		return
	}

	// iterate over the security definitions and find the first scheme with type OAuth2
	for defPair := Definitions.First(); defPair != nil; defPair = defPair.Next() {
		definition := defPair.Value()
		if definition.Type == "oauth2" && definition.Scopes != nil {

			for scopePair := definition.Scopes.Values.First(); scopePair != nil; scopePair = scopePair.Next() {

				//append scope to the api security authentication oauth2 scopes
				apiSpecificationSpec.Oauth2Scopes = append(apiSpecificationSpec.Oauth2Scopes, scopePair.Key())
			}
		}
	}
}

func setSecuritySchemeValues(apiSpecificationSpec *roverv1.ApiSpecificationSpec, SecuritySchemes *orderedmap.Map[string, *v3.SecurityScheme]) {
	if SecuritySchemes.Len() == 0 {
		return
	}

	// iterate over the security schemes and find the first scheme with type OAuth2
	for schemePair := SecuritySchemes.First(); schemePair != nil; schemePair = schemePair.Next() {
		scheme := schemePair.Value()
		if scheme.Type == "oauth2" && scheme.Flows != nil {

			for scopePair := scheme.Flows.ClientCredentials.Scopes.First(); scopePair != nil; scopePair = scopePair.Next() {

				//append scope to the api security authentication oauth2 scopes
				apiSpecificationSpec.Oauth2Scopes = append(apiSpecificationSpec.Oauth2Scopes, scopePair.Key())
			}

		}
	}
}
