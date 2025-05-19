// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"fmt"
	"strings"

	"github.com/telekom/controlplane/common-server/internal/crd"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/template"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func AddPredefinedController(builder *DocumentBuilder, ctrl *server.PredefinedController, opts server.ControllerOpts) {

	gvr, _ := ctrl.Store.Info()
	crd, err := crd.Instance.ResolveCrd(gvr)
	if err != nil {
		panic(err)
	}

	_, err = builder.AddSchemaBytes("Metadata", []byte(metadataSchema))
	if err != nil {
		panic(err)
	}

	_, err = builder.AddSchemaBytes(crd.GVK.Kind+"Spec", []byte(crd.SpecDefinition))
	if err != nil {
		panic(err)
	}

	_, err = builder.AddSchemaBytes(crd.GVK.Kind+"Status", []byte(crd.StatusDefinition))
	if err != nil {
		panic(err)
	}

	caser := cases.Title(language.AmericanEnglish)
	tagName := caser.String(ctrl.Name)
	operationName := caser.String(gvr.Resource) + caser.String(ctrl.Name)

	namespaceParameter := NewParameterBuilder().
		SetName("namespace").
		SetIn("path").
		SetRequired(true).
		SetSchema(NewSchemaItemType("string")).
		Build()

	nameParameter := NewParameterBuilder().
		SetName("name").
		SetIn("path").
		SetRequired(true).
		SetSchema(NewSchemaItemType("string")).
		Build()

	if ctrl.OnlyFilter() {
		// List Operation
		params := GetFilterParameters(ctrl.Filters)
		opBuilder := NewOperationBuilder()
		for _, param := range params {
			opBuilder.AddParameter(param)
		}

		builder.AddNewPath(opts.Prefix+"/"+ctrl.Name).
			SetOperation("GET", opBuilder.
				SetMeta("List operation for "+ctrl.Name, operationName).
				AddTags(crd.GVK.Kind, tagName).
				SetJsonResponse("200", NewSchemaItemSchemaRef(crd.GVK.Kind)).
				SetJsonResponse(NewSchemaItemResponseRef("400")).
				SetJsonResponse(NewSchemaItemResponseRef("500")).
				Build()).
			Build()

	}

	if ctrl.OnlyPatch() {
		// Patch Operation

		schemaRef := operationName + "PatchRequest"
		_, err := builder.AddSchemaBytes(schemaRef, GetPatchBodySchema(ctrl.Patches))
		if err != nil {
			panic(err)
		}

		builder.AddNewPath(opts.Prefix+"/"+ctrl.Name+"/{namespace}/{name}").
			SetOperation("PATCH", NewOperationBuilder().
				SetMeta("Patch operation for "+ctrl.Name, operationName).
				AddParameter(namespaceParameter).
				AddParameter(nameParameter).
				AddTags(crd.GVK.Kind, tagName).
				SetCustomRequestBody("application/json-patch+json", NewSchemaItemSchemaRef(schemaRef)).
				SetJsonResponse("201", NewSchemaItemSchemaRef(crd.GVK.Kind)).
				SetJsonResponse(NewSchemaItemResponseRef("400")).
				SetJsonResponse(NewSchemaItemResponseRef("500")).
				Build()).
			Build()

	}

	if ctrl.OnlyFindMatch() {
		// Find Match Operation
		opBuilder := NewOperationBuilder()

		filterParams := GetFilterParameters(ctrl.Filters)

		for _, param := range filterParams {
			opBuilder.AddParameter(param)
		}

		schemaRef := operationName + "PatchRequest"
		_, err := builder.AddSchemaBytes(schemaRef, GetPatchBodySchema(ctrl.Patches))
		if err != nil {
			panic(err)
		}

		builder.AddNewPath(opts.Prefix+"/"+ctrl.Name).
			SetOperation("PATCH", opBuilder.
				SetMeta("Find match operation for "+ctrl.Name, operationName).
				AddTags(crd.GVK.Kind, tagName).
				SetCustomRequestBody("application/json-patch+json", NewSchemaItemSchemaRef(schemaRef)).
				SetJsonResponse("201", NewSchemaItemSchemaRef(crd.GVK.Kind)).
				SetJsonResponse(NewSchemaItemResponseRef("400")).
				SetJsonResponse(NewSchemaItemResponseRef("500")).
				Build()).
			Build()

	}
}

func GetFilterParameters(filters []store.Filter) []*Parameter {
	var parameters []*Parameter
	for _, filter := range filters {
		placeholders := template.New(filter.Value).GetAllPlaceholders()
		for _, placeholder := range placeholders {
			parameters = append(parameters, NewParameterBuilder().
				SetName(placeholder).
				SetIn("query").
				SetDescription(fmt.Sprintf("Filter by %s", placeholder)).
				SetRequired(true).
				SetSchema(NewSchemaItemType("string")).
				Build())
		}
	}
	return parameters
}

func GetPatchBodySchema(patches []store.Patch) []byte {
	fields := []string{}
	fieldKeys := []string{}

	for _, patch := range patches {

		placeholders := template.New(patch.Value).GetAllPlaceholders()
		for _, placeholder := range placeholders {
			fields = append(fields, fmt.Sprintf(`"%s": {"type": "string"}`, placeholder))
			fieldKeys = append(fieldKeys, placeholder)
		}
	}

	requiredFields := fmt.Sprintf(`["%s"]`, strings.Join(fieldKeys, `", "`))
	return []byte(fmt.Sprintf(`{"type": "object", "properties": {%s}, "required": %s}`, strings.Join(fields, ","), requiredFields))
}
