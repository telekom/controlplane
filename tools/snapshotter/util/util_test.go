// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/telekom/controlplane/tools/snapshotter/util"
)

func TestDeepSort(t *testing.T) {

	type testStruct struct {
		Slice     []string
		Map       map[string]any
		SubStruct *testStruct
	}

	ts := []testStruct{
		{
			Slice: []string{"banana", "apple", "cherry"},
			Map: map[string]any{
				"c": 3,
				"b": 2,
				"a": map[string]any{
					"items": []string{"mouse", "cat", "dog"},
				},
			},
			SubStruct: &testStruct{
				Slice: []string{"baz", "bar", "foo"},
				Map: map[string]any{
					"z": 26,
					"x": 24,
					"y": 25,
				},
				SubStruct: nil,
			},
		},
	}

	util.DeepSort(ts)

	output, _ := yaml.Marshal(ts)
	expected := `- slice:
  - apple
  - banana
  - cherry
  map:
    a:
      items:
      - cat
      - dog
      - mouse
    b: 2
    c: 3
  substruct:
    slice:
    - bar
    - baz
    - foo
    map:
      x: 24
      "y": 25
      z: 26
    substruct: null
`

	assert.YAMLEq(t, expected, string(output), "DeepSort should sort the struct fields and slice elements correctly")
}
