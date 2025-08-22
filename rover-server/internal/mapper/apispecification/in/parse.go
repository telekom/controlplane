// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"net/url"
	"strings"

	"context"

	"github.com/pb33f/libopenapi"
	v2 "github.com/pb33f/libopenapi/datamodel/high/v2"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"gopkg.in/yaml.v3"
)

func parseSpecification(ctx context.Context, spec string) (*roverv1.ApiSpecificationSpec, error) {
	apiSpecificationSpec := &roverv1.ApiSpecificationSpec{
		XVendor:      false,
		Category:     "other",
		Oauth2Scopes: []string{},
	}
	log := log.FromContext(ctx)

	rawSpec := []byte(strings.TrimSpace(spec))
	document, err := libopenapi.NewDocument(rawSpec)
	if err != nil {
		return nil, errors.Wrap(err, parseErr)
	}

	version := document.GetVersion()
	if strings.HasPrefix(version, "2.") {
		model, errs := document.BuildV2Model()
		if errs != nil {
			log.Info("failed to build v2 model", zap.Errors("errors", errs))
			return nil, errors.New(parseErr + ": failed to build v2 model")
		}

		apiSpecificationSpec.ApiName = makeName(model.Model.BasePath)
		apiSpecificationSpec.BasePath = model.Model.BasePath
		apiSpecificationSpec.Version = model.Model.Info.Version
		setExtensionValues(apiSpecificationSpec, model.Model.Info.Extensions)

		if model.Model.SecurityDefinitions != nil {
			setSecurityDefinitionsValues(apiSpecificationSpec, model.Model.SecurityDefinitions.Definitions)
		}

		return apiSpecificationSpec, nil
	}

	if strings.HasPrefix(version, "3.") {
		model, errs := document.BuildV3Model()
		if errs != nil {
			log.Info("failed to build v3 model", zap.Errors("errors", errs))
			return nil, errors.New(parseErr + ": failed to build v3 model")
		}

		if len(model.Model.Servers) == 0 {
			return nil, errors.New(parseErr + ": there are no servers in the spec")
		}

		path, err := getPathFromURL(model.Model.Servers[0].URL)
		if err != nil {
			return nil, errors.Wrap(err, "failed to make name from url")
		}

		apiSpecificationSpec.ApiName = makeName(path)
		apiSpecificationSpec.BasePath = path
		apiSpecificationSpec.Version = model.Model.Info.Version

		setExtensionValues(apiSpecificationSpec, model.Model.Info.Extensions)

		if model.Model.Components != nil {
			setSecuritySchemeValues(apiSpecificationSpec, model.Model.Components.SecuritySchemes)
		}

		return apiSpecificationSpec, nil
	}

	return nil, errors.New(parseErr + ": unsupported specification version")
}

func makeName(basePath string) string {
	return strings.Trim(strings.ReplaceAll(basePath, "/", "-"), "-")
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
	value, ok := extensionMap.Get("x-category")
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
