// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification

import (
	"context"
	"net/url"
	"strings"

	"github.com/pb33f/libopenapi"
	v2 "github.com/pb33f/libopenapi/datamodel/high/v2"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	ParseErr = "failed to parse specification"
)

func ParseSpecification(ctx context.Context, spec string) (*apiapi.Api, error) {
	api := &apiapi.Api{}
	api.Spec = apiapi.ApiSpec{
		XVendor:  false,
		Category: "other",
		Security: apiapi.Security{
			M2M: &apiapi.Machine2MachineAuthentication{
				Scopes: []string{},
			},
		},
	}
	log := log.FromContext(ctx)

	rawSpec := []byte(strings.TrimSpace(spec))
	document, err := libopenapi.NewDocument(rawSpec)
	if err != nil {
		return nil, errors.Wrap(err, ParseErr)
	}

	version := document.GetVersion()
	if strings.HasPrefix(version, "2.") {
		model, errs := document.BuildV2Model()
		if errs != nil {
			log.Info("failed to build v2 model", zap.Errors("errors", errs))
			return nil, errors.New(ParseErr + ": failed to build v2 model")
		}

		api.Name = MakeName(model.Model.BasePath)
		api.Spec.Name = api.Name
		api.Spec.BasePath = model.Model.BasePath
		api.Spec.Version = model.Model.Info.Version

		setExtensionValues(api, model.Model.Info.Extensions)

		if model.Model.SecurityDefinitions != nil {
			setSecurityDefinitionsValues(api, model.Model.SecurityDefinitions.Definitions)
		}

		return api, nil
	}

	if strings.HasPrefix(version, "3.") {
		model, errs := document.BuildV3Model()
		if errs != nil {
			log.Info("failed to build v3 model", zap.Errors("errors", errs))
			return nil, errors.New(ParseErr + ": failed to build v3 model")
		}

		if len(model.Model.Servers) == 0 {
			return nil, errors.New(ParseErr + ": there are no servers in the spec")
		}

		path, err := GetPathFromURL(model.Model.Servers[0].URL)
		if err != nil {
			return nil, errors.Wrap(err, "failed to make name from url")
		}

		api.Name = MakeName(path)
		api.Spec.Name = api.Name
		api.Spec.BasePath = path
		api.Spec.Version = model.Model.Info.Version

		setExtensionValues(api, model.Model.Info.Extensions)

		if model.Model.Components != nil {
			setSecuritySchemeValues(api, model.Model.Components.SecuritySchemes)
		}

		return api, nil
	}

	return nil, errors.New(ParseErr + ": unsupported specification version")
}

func MakeName(basePath string) string {
	return strings.Trim(strings.ReplaceAll(basePath, "/", "-"), "-")
}

func GetPathFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse url")
	}

	return u.Path, nil
}

func setExtensionValues(api *apiapi.Api, extensionMap *orderedmap.Map[string, *yaml.Node]) {
	if extensionMap == nil {
		return
	}
	value, ok := extensionMap.Get("x-category")
	if ok {
		api.Spec.Category = value.Value
	}
	value, ok = extensionMap.Get("x-vendor")
	if ok {
		api.Spec.XVendor = value.Value == "true"
	}
}

func setSecurityDefinitionsValues(api *apiapi.Api, Definitions *orderedmap.Map[string, *v2.SecurityScheme]) {
	if Definitions.Len() == 0 {
		return
	}

	// iterate over the security definitions and find the first scheme with type OAuth2
	for defPair := Definitions.First(); defPair != nil; defPair = defPair.Next() {
		definition := defPair.Value()
		if definition.Type == "oauth2" && definition.Scopes != nil {

			for scopePair := definition.Scopes.Values.First(); scopePair != nil; scopePair = scopePair.Next() {

				//append scope to the api security authentication oauth2 scopes
				api.Spec.SubscriberSecurity.M2M.Scopes = append(api.Spec.SubscriberSecurity.M2M.Scopes, scopePair.Key())
			}
		}
	}
}

func setSecuritySchemeValues(api *apiapi.Api, SecuritySchemes *orderedmap.Map[string, *v3.SecurityScheme]) {
	if SecuritySchemes.Len() == 0 {
		return
	}

	// iterate over the security schemes and find the first scheme with type OAuth2
	for schemePair := SecuritySchemes.First(); schemePair != nil; schemePair = schemePair.Next() {
		scheme := schemePair.Value()
		if scheme.Type == "oauth2" && scheme.Flows != nil {

			for scopePair := scheme.Flows.ClientCredentials.Scopes.First(); scopePair != nil; scopePair = scopePair.Next() {

				//append scope to the api security authentication oauth2 scopes
				api.Spec.SubscriberSecurity.M2M.Scopes = append(api.Spec.SubscriberSecurity.M2M.Scopes, scopePair.Key())
			}

		}
	}
}
