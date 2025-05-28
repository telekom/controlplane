// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"fmt"
	"strings"
)

// StringMap is a map of strings that encodes to a JSON array of strings
// with the format [key1:value1, key2:value2]
type StringMap struct {
	items map[string]string
}

func New() *StringMap {
	return &StringMap{items: make(map[string]string)}
}

func (m *StringMap) AddKV(key, value string) {
	m.items[key] = value
}

func (m *StringMap) Add(value string) {
	parts := strings.Split(value, ":")
	m.items[parts[0]] = parts[1]
}

func (m *StringMap) RemoveK(key, value string) {
	delete(m.items, key)
}

func (m *StringMap) Remove(value string) {
	parts := strings.Split(value, ":")
	delete(m.items, parts[0])
}

func (m *StringMap) Clear() {
	m.items = make(map[string]string)
}

func (m *StringMap) Contains(key string) bool {
	if _, contains := m.items[key]; !contains {
		return false
	}
	return true
}

func (m *StringMap) Get(key string) string {
	if m.Contains(key) {
		return m.items[key]
	}
	return ""
}

// MarshalJSON encodes the map into a format like ["key1:value1", "key2:value2"]
func (m *StringMap) MarshalJSON() ([]byte, error) {
	if len(m.items) == 0 {
		return []byte("[]"), nil
	}
	result := "["
	for k, v := range m.items {
		result += fmt.Sprintf("\"%s:%s\",", k, v)
	}
	result = result[:len(result)-1] + "]" // remove the last comma and add the closing bracket
	return []byte(result), nil
}

// UnmarshalJSON decodes a string like ["key1:value1","key2:value2"] into a map
// ! It is mandatory that the json-array does not contain any spaces
func (m *StringMap) UnmarshalJSON(b []byte) error {
	if m.items == nil {
		m.items = make(map[string]string)
	}
	// Remove the brackets
	b = b[1 : len(b)-1]
	if len(b) == 0 {
		return nil
	}
	// Split the string into key-value pairs
	pairs := strings.Split(string(b), ",")
	for _, pair := range pairs {
		kv := strings.Split(strings.Trim(pair, "\""), ":")
		m.items[kv[0]] = kv[1]
	}
	return nil
}
