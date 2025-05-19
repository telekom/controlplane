package filter

import (
	"bytes"

	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/tidwall/gjson"
)

type FilterFunc func([]byte) bool

func NewFilterFuncs(filters []store.Filter) FilterFunc {
	funcs := make([]FilterFunc, 0, len(filters))
	for _, filter := range filters {
		funcs = append(funcs, NewFilterFunc(filter))
	}
	return And(funcs...)
}

func NewFilterFunc(filter store.Filter) FilterFunc {
	switch filter.Op {
	case store.OpEqual:
		return JsonPathFilterValue(filter.Path, NewSimple(filter.Value))
	case store.OpNotEqual:
		return JsonPathFilterValue(filter.Path, NotEq(NewSimple(filter.Value)))
	case store.OpRegex:
		return JsonPathFilterValue(filter.Path, NewRegex(filter.Value))
	case store.OpFullText:
		return FullTextFilter(filter.Value)
	default:
		return JsonPathFilter(filter.Path)
	}
}

func NopFilter(data []byte) bool {
	return true
}

func JsonPathFilter(path string) FilterFunc {
	return JsonPathFilterValue(path, nil)
}

func FullTextFilter(text string) FilterFunc {
	return func(data []byte) bool {
		return bytes.Contains(data, []byte(text))
	}
}

func JsonPathFilterValue(path string, eq Equality) FilterFunc {
	return func(data []byte) bool {
		res := gjson.GetBytes(data, path)
		if !res.Exists() {
			return false
		}
		if res.IsArray() && len(res.Array()) == 0 {
			return false
		}
		if len(res.Array()) == 1 {
			res = res.Array()[0]
		}
		if eq != nil {
			return eq.Equal(res.Value())
		}

		return true
	}
}

func Or(filters ...FilterFunc) FilterFunc {
	return func(data []byte) bool {
		for _, f := range filters {
			if f(data) {
				return true
			}
		}
		return false
	}
}

func And(filters ...FilterFunc) FilterFunc {
	return func(data []byte) bool {
		for _, f := range filters {
			if !f(data) {
				return false
			}
		}
		return true
	}
}

func Not(filter FilterFunc) FilterFunc {
	return func(data []byte) bool {
		return !filter(data)
	}
}
