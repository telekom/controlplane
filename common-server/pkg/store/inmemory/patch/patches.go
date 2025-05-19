// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package patch

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/telekom/controlplane/common-server/pkg/store"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type PatchFunc func([]byte) ([]byte, error)

func NewPatchFuncs(patches []store.Patch) PatchFunc {
	patchFuncs := make([]PatchFunc, len(patches))
	for i, patch := range patches {
		patchFuncs[i] = NewPatchFunc(patch)
	}
	return PatchAll(patchFuncs)
}

func NewPatchFunc(patch store.Patch) PatchFunc {
	switch patch.Op {
	case store.OpAdd:
		return AddPatch(patch.Path, patch.Value)
	case store.OpRemove:
		return RemovePatch(patch.Path)
	case store.OpReplace:
		return ReplacePatch(patch.Path, patch.Value)
	default:
		return UnsupportedPatch(patch.Op)
	}
}

func UnsupportedPatch(op store.PatchOp) PatchFunc {
	return func(data []byte) ([]byte, error) {
		return nil, fmt.Errorf("unsupported patch operation '%s'", op)
	}
}

func NopPatch(data []byte) ([]byte, error) {
	return data, nil
}

func AddPatch(path string, value interface{}) PatchFunc {
	return func(data []byte) ([]byte, error) {
		res := gjson.GetBytes(data, path)
		if !res.Exists() {
			return sjson.SetBytes(data, path, []any{value})
		}
		if res.IsArray() {
			return sjson.SetBytes(data, path+".-1", value)
		}
		return nil, fmt.Errorf("cannot patch '%s': already exists and is not an array", path)
	}
}

func RemovePatch(path string) PatchFunc {
	return func(data []byte) ([]byte, error) {
		return sjson.SetBytes(data, path, nil)
	}
}

func ReplacePatch(path string, value interface{}) PatchFunc {
	return func(data []byte) ([]byte, error) {
		res := gjson.GetBytes(data, path)
		if res.Exists() {
			actual := reflect.ValueOf(value).Type()
			expected := res.Type.String()
			if expected == "JSON" && actual.Kind() == reflect.Map {
				return sjson.SetBytes(data, path, value)
			}
			if expected == "JSON" && actual.Kind() == reflect.Slice {
				return sjson.SetBytes(data, path, value)
			}
			if expected == "Number" && isNumeric(actual.Kind()) {
				return sjson.SetBytes(data, path, value)
			}
			if !strings.EqualFold(actual.String(), expected) {
				return nil, fmt.Errorf("cannot patch '%s': expected type '%s' but got '%s'", path, expected, actual)
			}
		}
		return sjson.SetBytes(data, path, value)
	}
}

func PatchAll(fns []PatchFunc) PatchFunc {
	return func(data []byte) ([]byte, error) {
		for _, fn := range fns {
			var err error
			data, err = fn(data)
			if err != nil {
				return nil, err
			}
		}
		return data, nil
	}
}

func isNumeric(v reflect.Kind) bool {
	switch v {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}
