// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ = Describe("Util", func() {

	Context("Removing Nil Fields", func() {

		It("should not modify an empty object", func() {
			// given
			obj := map[string]any{}

			// when
			types.RemoveNilFields(obj, 10)

			// then
			Expect(obj).To(BeEmpty())
		})

		It("should not modify an object with no nil fields", func() {
			// given
			obj := map[string]any{
				"field1": "value1",
				"field2": 42,
				"field3": true,
			}

			// when
			types.RemoveNilFields(obj, 10)

			// then
			Expect(obj).To(Equal(map[string]any{
				"field1": "value1",
				"field2": 42,
				"field3": true,
			}))
		})

		It("should remove all nil fields from an object", func() {

			// given
			obj := map[string]any{
				"field1": "value1",
				"field2": nil,
				"field3": map[string]any{
					"subField1": nil,
					"subField2": "value2",
				},
				"field4": []any{
					map[string]any{"listField1": nil, "listField2": "listValue"},
				},
				"field5": []any{},
				"field6": map[string]any{},
			}

			// when
			types.RemoveNilFields(obj, 10)

			// then

			Expect(obj).To(Equal(map[string]any{
				"field1": "value1",
				"field3": map[string]any{
					"subField2": "value2",
				},
				"field4": []any{
					map[string]any{"listField2": "listValue"},
				},
			}))
		})

		It("should quit after reaching max depth", func() {
			// given
			obj := map[string]any{
				"field1": "value1",
				"field2": nil,
				"field3": map[string]any{
					"subField1": nil,
					"subField2": map[string]any{
						"deepField1": nil,
						"deepField2": "deepValue",
					},
				},
			}

			// when
			types.RemoveNilFields(obj, 1)

			// then
			Expect(obj).To(Equal(map[string]any{
				"field1": "value1",
				"field3": map[string]any{
					"subField1": nil,
					"subField2": map[string]any{
						"deepField1": nil,
						"deepField2": "deepValue",
					},
				},
			}))
		})
	})

})
