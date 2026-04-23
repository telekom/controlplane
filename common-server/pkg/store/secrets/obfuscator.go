// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ Replacer = &Obfuscator{}

type Obfuscator struct {
	placeholder string
}

func NewObfuscator() *Obfuscator {
	return &Obfuscator{
		placeholder: strings.Repeat("*", 10),
	}
}

func (o *Obfuscator) ReplaceAll(ctx context.Context, obj any, jsonPaths []string) (any, error) {
	if obj == nil {
		return nil, nil
	}
	if len(jsonPaths) == 0 {
		return obj, nil
	}

	b, ok := obj.([]byte)
	if ok {
		return o.ReplaceAllFromBytes(ctx, b, jsonPaths)
	}
	str, ok := obj.(string)
	if ok {
		b, err := o.ReplaceAllFromBytes(ctx, []byte(str), jsonPaths)
		if b != nil {
			return string(b), err
		}
		return nil, err
	}
	m, ok := obj.(map[string]any)
	if ok {
		return o.ReplaceAllFromMap(ctx, m, jsonPaths)
	}
	u, ok := obj.(*unstructured.Unstructured)
	if ok {
		m, err := o.ReplaceAllFromMap(ctx, u.UnstructuredContent(), jsonPaths)
		if err != nil {
			return nil, errors.Wrap(err, "failed to replace all from unstructured")
		}
		u.SetUnstructuredContent(m)
		return u, nil
	}

	b, err := sonic.Marshal(obj)
	if err == nil {
		b, err = o.ReplaceAllFromBytes(ctx, b, jsonPaths)
		if err != nil {
			return nil, errors.Wrap(err, "failed to obfuscate from json")
		}
		err = sonic.Unmarshal(b, &obj)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal obfuscated json")
		}
		return obj, nil
	}

	return nil, fmt.Errorf("unsupported type %T", obj)
}

func (o *Obfuscator) ReplaceAllFromBytes(ctx context.Context, b []byte, jsonPaths []string) ([]byte, error) {
	for _, jsonPath := range jsonPaths {
		var err error
		b, err = sjson.SetBytes(b, jsonPath, []byte(o.placeholder))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to set json path %s", jsonPath)
		}
	}
	return b, nil
}

func (o *Obfuscator) ReplaceAllFromMap(ctx context.Context, m map[string]any, jsonPaths []string) (map[string]any, error) {
	for _, jsonPath := range jsonPaths {
		parts := strings.Split(jsonPath, ".")
		err := unstructured.SetNestedField(m, o.placeholder, parts...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to set secret value")
		}
	}
	return m, nil
}
