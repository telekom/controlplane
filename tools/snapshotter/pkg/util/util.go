// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"

	"go.uber.org/zap"
)

type Response interface {
	StatusCode() int
}

func MustBe2xx(res Response, context string) {
	if !Is2xx(res) {
		zap.L().Fatal("non-2xx response", zap.Int("status_code", res.StatusCode()), zap.String("context", context))
	}
}

func PrintErrResponse(body io.Reader, context string) {
	data, err := io.ReadAll(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to read error response body for %s: %v\n", context, err)
		return
	}
	fmt.Fprintf(os.Stderr, "Error response body for %s: %s\n", context, string(data))
}

func Is2xx(res Response) bool {
	return res.StatusCode() >= 200 && res.StatusCode() < 300
}

func Must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}

func EnvOrFail(envVar string) string {
	value := os.Getenv(envVar)
	if value == "" {
		fmt.Fprintf(os.Stderr, "Error: Environment variable %s is not set\n", envVar)
		os.Exit(1)
	}
	return value
}

func IfEmptyLoadEnv(value, envVar string) string {
	if value != "" {
		return value
	}
	return os.Getenv(envVar)
}

func IfEmptyLoadEnvOrFail(value, envVar string) string {
	if value != "" {
		return value
	}
	return EnvOrFail(envVar)
}

// DeepSort sorts the fields of a struct or elements of a slice recursively.
func DeepSort(obj any) {
	deepSort(reflect.ValueOf(obj))
}

func deepSort(v reflect.Value) {
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if !v.IsNil() {
			deepSort(v.Elem())
		}
	case reflect.Struct:
		for i := range v.NumField() {
			deepSort(v.Field(i))
		}
	case reflect.Slice:
		if v.Len() == 0 {
			return
		}

		elKind := v.Type().Elem().Kind()
		if elKind == reflect.String {
			slices.Sort(v.Interface().([]string))
		}
		if elKind == reflect.Int {
			slices.Sort(v.Interface().([]int))
		}
		if elKind == reflect.Interface {
			// Sort interfaces by string representation
			slices.SortFunc(v.Interface().([]any), func(a, b any) int {
				return cmp.Compare(fmt.Sprintf("%v", a), fmt.Sprintf("%v", b))
			})
		}

		if elKind == reflect.Slice || elKind == reflect.Map || elKind == reflect.Struct {
			// Recursively sort elements
			for i := range v.Len() {
				deepSort(v.Index(i))
			}
		}

	case reflect.Map:
		// Sort map values recursively
		for _, key := range v.MapKeys() {
			val := v.MapIndex(key)
			deepSort(val)
		}
	}
}
