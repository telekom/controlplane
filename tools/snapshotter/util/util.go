// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"os"
	"reflect"
	"sort"
)

type Response interface {
	StatusCode() int
}

func MustBe2xx(res Response, context string) {
	if res.StatusCode() < 200 || res.StatusCode() >= 300 {
		fmt.Fprintf(os.Stderr, "Error: %s returned status code %d\n", context, res.StatusCode())
		os.Exit(1)
	}
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
		// Sort slice if element is comparable (int, string, etc.)
		if v.Len() == 0 {
			return
		}
		elemKind := v.Type().Elem().Kind()
		switch elemKind {
		case reflect.Int:
			sort.Slice(v.Interface(), func(i, j int) bool {
				return v.Index(i).Int() < v.Index(j).Int()
			})
		case reflect.String:
			sort.Slice(v.Interface(), func(i, j int) bool {
				return v.Index(i).String() < v.Index(j).String()
			})
		case reflect.Struct, reflect.Ptr:
			// Recursively sort elements
			for i := range v.Len() {
				deepSort(v.Index(i))
			}
		}
	}
}
