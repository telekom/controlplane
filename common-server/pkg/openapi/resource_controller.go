// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"fmt"
	"strings"

	"github.com/telekom/controlplane/common-server/internal/crd"
	"github.com/telekom/controlplane/common-server/pkg/server"
)

func AddResourceController(builder *DocumentBuilder, ctrl *server.ResourceController, opts server.ControllerOpts) {

	gvr, _ := ctrl.Store.Info()
	crd, err := crd.Instance.ResolveCrd(gvr)
	if err != nil {
		panic(err)
	}

	_, err = builder.AddSchemaBytes("ApiProblem", []byte(ApiProblemSchema))
	if err != nil {
		panic(err)
	}

	builder.AddResponse("400", NewSchemaItemSchemaRef("ApiProblem"))
	builder.AddResponse("500", NewSchemaItemSchemaRef("ApiProblem"))

	_, err = builder.AddSchemaBytes("Metadata", []byte(metadataSchema))
	if err != nil {
		panic(err)
	}
	_, err = builder.AddSchemaBytes("PatchRequestBody", []byte(patchRequestBodySchema))
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

	_, err = builder.AddSchemaBytes(crd.GVK.Kind, []byte(fmt.Sprintf(crdSchema, componentsSchemaPath+"Metadata", componentsSchemaPath+crd.GVK.Kind+"Spec", componentsSchemaPath+crd.GVK.Kind+"Status")))
	if err != nil {
		panic(err)
	}

	filterParameter := NewParameterBuilder().
		SetName("filter").
		SetIn("query").
		SetDescription("Filter query").
		SetSchema(NewSchemaItemType("string")).
		Build()

	prefixParameter := NewParameterBuilder().
		SetName("prefix").
		SetIn("query").
		SetDescription("Prefix").
		SetSchema(NewSchemaItemType("string")).
		Build()

	limitParameter := NewParameterBuilder().
		SetName("limit").
		SetIn("query").
		SetDescription("Limit").
		SetSchema(NewSchemaItemType("integer")).
		Build()

	cursorParameter := NewParameterBuilder().
		SetName("cursor").
		SetIn("query").
		SetDescription("Cursor").
		SetSchema(NewSchemaItemType("string")).
		Build()

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

	// List Operation
	listPath := NewPathItemBuilder()

	if opts.IsAllowed("GET") {
		listPath.SetOperation("GET", NewOperationBuilder().
			AddParameter(filterParameter).
			AddParameter(prefixParameter).
			AddParameter(limitParameter).
			AddParameter(cursorParameter).
			AddTags(crd.GVK.Kind).
			SetMeta("List all "+crd.GVK.Kind+" resources", fmt.Sprintf("list-%s", strings.ToLower(crd.GVK.Kind))).
			SetDescription("List all "+crd.GVK.Kind+" resources").
			SetJsonResponse("200", NewArrayOfSchemaItemWithRef(NewSchemaItemSchemaRef(crd.GVK.Kind))).
			SetJsonResponse(NewSchemaItemResponseRef("400")).
			SetJsonResponse(NewSchemaItemResponseRef("500")).
			Build())
	}

	if opts.IsAllowed("POST") {
		listPath.SetOperation("POST", NewOperationBuilder().
			AddTags(crd.GVK.Kind).
			SetMeta("Create a new "+crd.GVK.Kind+" resource", fmt.Sprintf("create-%s", strings.ToLower(crd.GVK.Kind))).
			SetDescription("Create a new "+crd.GVK.Kind+" resource").
			SetJsonResponse("201", NewSchemaItemSchemaRef(crd.GVK.Kind)).
			SetJsonResponse(NewSchemaItemResponseRef("400")).
			SetJsonResponse(NewSchemaItemResponseRef("500")).
			SetJsonRequestBody(NewSchemaItemSchemaRef(crd.GVK.Kind)).
			Build())
	}

	builder.AddPath(opts.Prefix, listPath.Build())

	// Single Operation
	singlePath := NewPathItemBuilder()

	if opts.IsAllowed("GET") {
		singlePath.SetOperation("GET", NewOperationBuilder().
			AddParameter(namespaceParameter).
			AddParameter(nameParameter).
			AddTags(crd.GVK.Kind).
			SetMeta("Get a "+crd.GVK.Kind+" resource", fmt.Sprintf("get-%s", strings.ToLower(crd.GVK.Kind))).
			SetDescription("Get a "+crd.GVK.Kind+" resource").
			SetJsonResponse("200", NewSchemaItemSchemaRef(crd.GVK.Kind)).
			SetJsonResponse(NewSchemaItemResponseRef("400")).
			SetJsonResponse(NewSchemaItemResponseRef("500")).
			Build())
	}

	if opts.IsAllowed("PATCH") {
		singlePath.SetOperation("PATCH", NewOperationBuilder().
			AddParameter(namespaceParameter).
			AddParameter(nameParameter).
			AddTags(crd.GVK.Kind).
			SetMeta("Patch a "+crd.GVK.Kind+" resource", fmt.Sprintf("patch-%s", strings.ToLower(crd.GVK.Kind))).
			SetDescription("Patch a "+crd.GVK.Kind+" resource").
			SetJsonResponse("201", NewSchemaItemSchemaRef(crd.GVK.Kind)).
			SetCustomRequestBody("application/json-patch+json", NewSchemaItemSchemaRef("PatchRequestBody")).
			SetJsonResponse(NewSchemaItemResponseRef("400")).
			SetJsonResponse(NewSchemaItemResponseRef("500")).
			Build())
	}

	if opts.IsAllowed("PUT") {
		singlePath.SetOperation("PUT", NewOperationBuilder().
			AddParameter(namespaceParameter).
			AddParameter(nameParameter).
			AddTags(crd.GVK.Kind).
			SetMeta("Update a "+crd.GVK.Kind+" resource", fmt.Sprintf("update-%s", strings.ToLower(crd.GVK.Kind))).
			SetDescription("Update a "+crd.GVK.Kind+" resource").
			SetJsonResponse("201", NewSchemaItemSchemaRef(crd.GVK.Kind)).
			SetJsonResponse(NewSchemaItemResponseRef("400")).
			SetJsonResponse(NewSchemaItemResponseRef("500")).
			SetJsonRequestBody(NewSchemaItemSchemaRef(crd.GVK.Kind)).
			SetJsonResponse("409", NewSchemaItemSchemaRef("ApiProblem")).
			Build())
	}

	if opts.IsAllowed("DELETE") {
		singlePath.SetOperation("DELETE", NewOperationBuilder().
			AddParameter(namespaceParameter).
			AddParameter(nameParameter).
			AddTags(crd.GVK.Kind).
			SetMeta("Delete a "+crd.GVK.Kind+" resource", fmt.Sprintf("delete-%s", strings.ToLower(crd.GVK.Kind))).
			SetDescription("Delete a "+crd.GVK.Kind+" resource").
			SetJsonResponse("204", nil).
			SetJsonResponse(NewSchemaItemResponseRef("400")).
			SetJsonResponse(NewSchemaItemResponseRef("500")).
			Build())
	}

	builder.AddPath(opts.Prefix+"/{namespace}/{name}", singlePath.Build())

}
