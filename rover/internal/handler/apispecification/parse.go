// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apispecification

import (
	"context"
	"net/url"
	"strings"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
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
		Oauth2Scopes: []string{},
		XVendor:      false,
		Category:     "other",
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
		setSecurityValues(api, model.Model.Security)

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
		setSecurityValues(api, model.Model.Security)

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

func setSecurityValues(api *apiapi.Api, security []*base.SecurityRequirement) {
	if len(security) == 0 {
		return
	}
	api.Spec.Oauth2Scopes = security[0].Requirements.Value("oauth2")
}
