// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

func GetValue[T any](m map[string]any, key string) (vt T, ok bool) {
	v, ok := m[key]
	if !ok {
		return
	}
	return NotNilOfType[T](v)
}

func NotNilOfType[T any](value any) (v T, ok bool) {
	if value == nil {
		return
	}
	v, ok = value.(T)
	return
}
