package openapi

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

const (
	openApiVersion         = "3.0.0"
	componentsSchemaPath   = "#/components/schemas/"
	componentsResponsePath = "#/components/responses/"
)

type Info struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type PathItem struct {
	Get    *Operation `json:"get,omitempty"`
	Put    *Operation `json:"put,omitempty"`
	Post   *Operation `json:"post,omitempty"`
	Delete *Operation `json:"delete,omitempty"`
	Patch  *Operation `json:"patch,omitempty"`
}

type PathItemBuilder struct {
	pathItem        *PathItem
	onBuildCallback func(*PathItemBuilder)
}

func NewPathItemBuilder() *PathItemBuilder {
	return &PathItemBuilder{
		pathItem: &PathItem{},
	}
}

func (b *PathItemBuilder) SetOperation(method string, operation *Operation) *PathItemBuilder {
	switch strings.ToUpper(method) {
	case "GET":
		b.pathItem.Get = operation
	case "PUT":
		b.pathItem.Put = operation
	case "POST":
		b.pathItem.Post = operation
	case "DELETE":
		b.pathItem.Delete = operation
	case "PATCH":
		b.pathItem.Patch = operation
	default:
		panic("unsupported method")
	}
	return b
}

func (b *PathItemBuilder) Build() *PathItem {
	if b.onBuildCallback != nil {
		b.onBuildCallback(b)
	}
	return b.pathItem
}

type Operation struct {
	Tags        []string             `json:"tags,omitempty"`
	Summary     string               `json:"summary"`
	OperationId string               `json:"operationId"`
	Description string               `json:"description,omitempty"`
	Parameters  []*Parameter         `json:"parameters,omitempty"`
	RequestBody *RequestBody         `json:"requestBody,omitempty"`
	Responses   map[string]*Response `json:"responses,omitempty"`
}

type OperationBuilder struct {
	operation *Operation
}

func NewOperationBuilder() *OperationBuilder {
	return &OperationBuilder{
		operation: &Operation{},
	}
}

func (b *OperationBuilder) AddTags(tags ...string) *OperationBuilder {
	b.operation.Tags = tags
	return b
}

func (b *OperationBuilder) SetMeta(summary, operationId string) *OperationBuilder {
	b.operation.Summary = summary
	b.operation.OperationId = operationId
	return b
}

func (b *OperationBuilder) SetDescription(description string) *OperationBuilder {
	b.operation.Description = description
	return b
}

func (b *OperationBuilder) AddParameter(parameter *Parameter) *OperationBuilder {
	b.operation.Parameters = append(b.operation.Parameters, parameter)
	return b
}

func (b *OperationBuilder) SetJsonRequestBody(SchemaItem SchemaItem) *OperationBuilder {
	return b.SetCustomRequestBody("application/json", SchemaItem)
}

func (b *OperationBuilder) SetCustomRequestBody(contentType string, SchemaItem SchemaItem) *OperationBuilder {
	if SchemaItem == nil {
		return b
	}
	b.operation.RequestBody = &RequestBody{
		Content: map[string]*MediaType{
			contentType: {
				Schema: &SchemaItem,
			},
		},
	}
	return b
}

func (b *OperationBuilder) SetJsonRequestBodyBytes(SchemaItemBytes []byte) *OperationBuilder {
	if SchemaItemBytes == nil {
		return b
	}

	var SchemaItem SchemaItem
	err := json.Unmarshal(SchemaItemBytes, &SchemaItem)
	if err != nil {
		return nil
	}

	b.operation.RequestBody = &RequestBody{
		Content: map[string]*MediaType{
			"application/json": {
				Schema: &SchemaItem,
			},
		},
	}
	return b
}

func (b *OperationBuilder) SetJsonResponse(statusCode string, SchemaItem SchemaItem) *OperationBuilder {
	if statusCode == "" {
		statusCode = "default"
	}
	contentType := "application/json"

	statusCodeInt, err := strconv.Atoi(statusCode)
	if err != nil {
		panic(fmt.Sprintf("invalid status code: %s", statusCode))
	}
	if statusCodeInt >= 400 {
		contentType = "application/problem+json"
	}
	return b.SetCustomResponse(statusCode, contentType, SchemaItem)

}

func (b *OperationBuilder) SetCustomResponse(statusCode, contentType string, SchemaItem SchemaItem) *OperationBuilder {
	if b.operation.Responses == nil {
		b.operation.Responses = Responses{}
	}
	if b.operation.Responses == nil {
		b.operation.Responses = map[string]*Response{}
	}

	response := &Response{
		Description: fmt.Sprintf(`This is an automatically generated response for status code %s`, statusCode),
	}

	if SchemaItem != nil {
		response = &Response{
			Description: "OK",
			Content: map[string]*MediaType{
				contentType: {
					Schema: &SchemaItem,
				},
			},
		}
	}

	b.operation.Responses[statusCode] = response
	return b
}

func (b *OperationBuilder) SetJsonResponseBytes(statusCode string, SchemaItemBytes []byte) *OperationBuilder {
	var SchemaItem SchemaItem
	err := json.Unmarshal(SchemaItemBytes, &SchemaItem)
	if err != nil {
		return nil
	}

	return b.SetJsonResponse(statusCode, SchemaItem)
}

func (b *OperationBuilder) Build() *Operation {
	return b.operation
}

type Parameter struct {
	Name        string      `json:"name"`
	In          string      `json:"in"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Schema      *SchemaItem `json:"schema"`
}

type ParameterBuilder struct {
	parameter *Parameter
}

func NewParameterBuilder() *ParameterBuilder {
	return &ParameterBuilder{
		parameter: &Parameter{},
	}
}

func (b *ParameterBuilder) SetName(name string) *ParameterBuilder {
	b.parameter.Name = name
	return b
}

func (b *ParameterBuilder) SetIn(in string) *ParameterBuilder {
	b.parameter.In = in
	return b
}

func (b *ParameterBuilder) SetDescription(description string) *ParameterBuilder {
	b.parameter.Description = description
	return b
}

func (b *ParameterBuilder) SetRequired(required bool) *ParameterBuilder {
	b.parameter.Required = required
	return b
}

func (b *ParameterBuilder) SetSchema(SchemaItem SchemaItem) *ParameterBuilder {
	b.parameter.Schema = &SchemaItem
	return b
}

func (b *ParameterBuilder) Build() *Parameter {
	return b.parameter
}

type RequestBody struct {
	Content map[string]*MediaType `json:"content"`
}

type MediaType struct {
	Schema *SchemaItem `json:"schema"`
}

type Responses map[string]*Response

type Response struct {
	Description string                `json:"description,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

type Components struct {
	Responses map[string]SchemaItem `json:"responses,omitempty"`
	Schemas   map[string]SchemaItem `json:"schemas,omitempty"`
}

type SchemaItem map[string]any

func (s SchemaItem) AddAttribute(name string, value any) SchemaItem {
	if value == nil {
		return s
	}
	if reflect.ValueOf(value).IsZero() {
		return s
	}

	s[name] = value
	return s
}

func NewSchemaItemResponseRef(statusCode string) (string, SchemaItem) {
	return statusCode, SchemaItem{
		"$ref": componentsResponsePath + statusCode,
	}
}

func NewSchemaItemSchemaRef(name string) SchemaItem {
	return SchemaItem{
		"$ref": componentsSchemaPath + name,
	}
}

func NewSchemaItemType(schemaType string) SchemaItem {
	return SchemaItem{
		"type": schemaType,
	}
}

func NewArrayOfSchemaItemWithRef(ref SchemaItem) SchemaItem {
	return SchemaItem{
		"type":  "array",
		"items": ref,
	}
}

type Tag struct {
	Name string `json:"name"`
}

type Server struct {
	Url         string `json:"url"`
	Description string `json:"description"`
}

type Document struct {
	Openapi    string               `json:"openapi"`
	Info       *Info                `json:"info,omitempty"`
	Tags       []Tag                `json:"tags,omitempty"`
	Servers    []Server             `json:"servers,omitempty"`
	Paths      map[string]*PathItem `json:"paths,omitempty"`
	Components *Components          `json:"components,omitempty"`
}

func NewDocument() *Document {
	return &Document{
		Openapi: openApiVersion,
		Info:    &Info{},
		Paths:   map[string]*PathItem{},
		Components: &Components{
			Responses: map[string]SchemaItem{},
			Schemas:   map[string]SchemaItem{},
		},
	}
}

type DocumentBuilder struct {
	alreadyBuilt bool
	globalTags   map[string]bool // this is a set
	doc          *Document
}

func (b *DocumentBuilder) addGlobalTags(operations ...*Operation) {
	for _, operation := range operations {
		if operation == nil {
			continue
		}
		for _, tag := range operation.Tags {
			b.globalTags[tag] = true
		}
	}
}

func NewDocumentBuilder() *DocumentBuilder {
	return &DocumentBuilder{
		globalTags: make(map[string]bool),
		doc:        NewDocument(),
	}
}

func (b *DocumentBuilder) SetOpenapiVersion(version string) *DocumentBuilder {
	b.doc.Openapi = version
	return b
}

func (b *DocumentBuilder) NewInfo(title, description, version string) *DocumentBuilder {
	b.doc.Info.Title = title
	b.doc.Info.Description = description
	b.doc.Info.Version = version
	return b
}

func (b *DocumentBuilder) AddServer(url, description string) *DocumentBuilder {
	b.doc.Servers = append(b.doc.Servers, Server{Url: url, Description: description})
	return b
}

func (b *DocumentBuilder) AddResponse(statusCode string, schemaItem SchemaItem) *DocumentBuilder {
	b.doc.Components.Responses[statusCode] = schemaItem
	return b
}

func (b *DocumentBuilder) AddPath(path string, pathItem *PathItem) *DocumentBuilder {
	b.doc.Paths[path] = pathItem
	b.addGlobalTags(pathItem.Get, pathItem.Put, pathItem.Post, pathItem.Delete, pathItem.Patch)
	return b
}

func (b *DocumentBuilder) AddNewPath(path string) *PathItemBuilder {
	builder := NewPathItemBuilder()
	builder.onBuildCallback = func(pathBuilder *PathItemBuilder) {
		b.AddPath(path, pathBuilder.pathItem)
	}
	return builder
}

func (b *DocumentBuilder) AddSchemaItem(name string, schemaItem SchemaItem) *DocumentBuilder {
	b.doc.Components.Schemas[name] = schemaItem
	return b
}

func (b *DocumentBuilder) AddSchemaBytesOrDie(name string, schemaBytes []byte) *DocumentBuilder {
	b, err := b.AddSchemaBytes(name, schemaBytes)
	if err != nil {
		panic(err)
	}
	return b
}

func (b *DocumentBuilder) AddSchemaBytes(name string, schemaBytes []byte) (*DocumentBuilder, error) {
	var schema SchemaItem
	err := json.Unmarshal(schemaBytes, &schema)
	if err != nil {
		return b, err
	}
	return b.AddSchemaItem(name, schema), nil
}

func (b *DocumentBuilder) Build() *Document {
	if b.alreadyBuilt {
		return b.doc
	}
	b.alreadyBuilt = true
	b.doc.Tags = make([]Tag, 0, len(b.globalTags))
	for tag := range b.globalTags {
		b.doc.Tags = append(b.doc.Tags, Tag{Name: tag})
	}

	return b.doc
}

func (b *DocumentBuilder) BuildBytes() ([]byte, error) {
	return json.MarshalIndent(b.Build(), "", "  ")
}

func (d *Document) Write(path string) error {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}
