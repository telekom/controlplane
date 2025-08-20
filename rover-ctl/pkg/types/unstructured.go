// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

var _ Object = &UnstructuredObject{}

type UnstructuredObject struct {
	Properties map[string]any `json:"-" yaml:"-"`
	Content    map[string]any `json:",inline" yaml:",inline"`
}

func (o *UnstructuredObject) GetApiVersion() string {
	apiVersion := o.GetProperty("apiVersion")
	if apiVersion != nil {
		return apiVersion.(string)
	}
	if apiVersion, ok := o.Content["apiVersion"].(string); ok {
		return apiVersion
	}
	return ""
}

func (o *UnstructuredObject) SetApiVersion(version string) {
	o.Content["apiVersion"] = version
	o.SetProperty("apiVersion", version)
}

func (o *UnstructuredObject) GetKind() string {
	kind := o.GetProperty("kind")
	if kind != nil {
		return kind.(string)
	}

	if kind, ok := o.Content["kind"].(string); ok {
		return kind
	}
	return ""
}

func (o *UnstructuredObject) SetKind(kind string) {
	o.Content["kind"] = kind
	o.SetProperty("kind", kind)
}

func (o *UnstructuredObject) GetName() string {
	name := o.GetProperty("name")
	if name != nil {
		return name.(string)
	}
	if metadata, ok := o.Content["metadata"].(map[string]any); ok {
		if name, ok := metadata["name"].(string); ok {
			return name
		}
	}
	return ""
}

func (o *UnstructuredObject) SetName(name string) {
	if _, ok := o.Content["metadata"].(map[string]any); !ok {
		o.Content["metadata"] = map[string]any{}
	}
	o.Content["metadata"].(map[string]any)["name"] = name
	o.SetProperty("name", name)
}

func (o *UnstructuredObject) GetContent() map[string]any {
	return o.Content
}

func (o *UnstructuredObject) SetContent(content map[string]any) {
	o.Content = content
}

func (o *UnstructuredObject) GetProperty(name string) any {
	if value, exists := o.Properties[name]; exists {
		return value
	}
	return nil
}
func (o *UnstructuredObject) SetProperty(name string, value any) {
	if o.Properties == nil {
		o.Properties = make(map[string]any)
	}
	o.Properties[name] = value
}
