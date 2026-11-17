// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"fmt"

	"github.com/goccy/go-yaml"
)

var _ Object = &UnstructuredObject{}

// PropertyOrderedContent holds the source document in its original order: the
// value decoded with yaml.UseOrderedMap for YAML files or a json.RawMessage for
// JSON files.
const PropertyOrderedContent = "orderedContent"

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

// Ordered prepares a document decoded with yaml.UseOrderedMap for JSON: keys
// become strings, and keys repeated by YAML merge keys collapse like in a map
// (the last value wins, at the position of the first key).
func Ordered(v any) any {
	switch t := v.(type) {
	case yaml.MapSlice:
		out := make(yaml.MapSlice, 0, len(t))
		index := make(map[string]int, len(t))
		for _, item := range t {
			value := Ordered(item.Value)
			key := fmt.Sprint(item.Key)
			if i, ok := index[key]; ok {
				out[i].Value = value
				continue
			}
			index[key] = len(out)
			out = append(out, yaml.MapItem{Key: key, Value: value})
		}
		return out
	case []any:
		items := make([]any, len(t))
		for i, item := range t {
			items[i] = Ordered(item)
		}
		return items
	default:
		return v
	}
}

// ToMap converts a mapping decoded with yaml.UseOrderedMap into nested maps.
func ToMap(ordered yaml.MapSlice) map[string]any {
	m := make(map[string]any, len(ordered))
	for _, item := range ordered {
		m[fmt.Sprint(item.Key)] = toPlain(item.Value)
	}
	return m
}

func toPlain(v any) any {
	switch t := v.(type) {
	case yaml.MapSlice:
		return ToMap(t)
	case []any:
		items := make([]any, len(t))
		for i, item := range t {
			items[i] = toPlain(item)
		}
		return items
	default:
		return v
	}
}
