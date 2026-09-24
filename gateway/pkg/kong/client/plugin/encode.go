// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"encoding/json"
	"fmt"
	"slices"
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

func splitEntry(value string) (string, string, bool) {
	pos := strings.Index(value, ":")
	if pos < 0 {
		return "", "", false
	}
	return value[:pos], value[pos+1:], true
}

func (m *StringMap) Add(value string) {
	key, val, ok := splitEntry(value)
	if !ok {
		return
	}
	m.items[key] = val
}

func (m *StringMap) RemoveK(key, value string) {
	delete(m.items, key)
}

func (m *StringMap) Remove(value string) {
	if m == nil {
		return
	}
	if !strings.Contains(value, ":") {
		delete(m.items, value)
		return
	}
	key, _, ok := splitEntry(value)
	if !ok {
		return
	}
	delete(m.items, key)
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

// MarshalJSON encodes the map into a format like ["key1:value1", "key2:value2"].
// Entries are sorted because Go randomizes map iteration order: an unsorted
// array would differ on every call, which makes the Kong client see a config
// change on every reconciliation and write when nothing changed.
func (m *StringMap) MarshalJSON() ([]byte, error) {
	entries := make([]string, 0, len(m.items))
	for k, v := range m.items {
		entries = append(entries, k+":"+v)
	}
	slices.Sort(entries)
	return json.Marshal(entries)
}

// UnmarshalJSON decodes a string like ["key1:value1","key2:value2"] into a map
// ! It is mandatory that the json-array does not contain any spaces
func (m *StringMap) UnmarshalJSON(b []byte) error {
	if m.items == nil {
		m.items = make(map[string]string)
	}
	var pairs []string
	if err := json.Unmarshal(b, &pairs); err != nil {
		return err
	}
	for _, pair := range pairs {
		key, value, ok := splitEntry(pair)
		if !ok {
			return fmt.Errorf("invalid string map entry %q", pair)
		}
		m.items[key] = value
	}
	return nil
}
