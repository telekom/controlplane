// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

func CleanObject(o Object) {
	if o == nil {
		return
	}

	RemoveNilFields(o.GetContent())
}

func RemoveNilFields(m map[string]any) {
	for k, v := range m {
		if v == nil {
			delete(m, k)
			continue
		}
		switch vv := v.(type) {
		case map[string]any:
			RemoveNilFields(vv)
			if len(vv) == 0 {
				delete(m, k)
			}

		case []any:
			for _, item := range vv {
				if subMap, ok := item.(map[string]any); ok {
					RemoveNilFields(subMap)
				}
			}
			if len(vv) == 0 {
				delete(m, k)
			}
		}
	}
}
