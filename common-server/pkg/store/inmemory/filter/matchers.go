package filter

import (
	"regexp"
	"strconv"

	"github.com/bytedance/sonic"
)

type Equality interface {
	Equal(any) bool
}

type not struct {
	eq Equality
}

func NotEq(eq Equality) Equality {
	return &not{
		eq: eq,
	}
}

func (n *not) Equal(value any) bool {
	return !n.eq.Equal(value)
}

type Regex struct {
	pattern *regexp.Regexp
}

func NewRegex(pattern string) *Regex {
	return &Regex{
		pattern: regexp.MustCompile(pattern),
	}
}
func (r *Regex) Equal(value any) bool {
	switch value := value.(type) {
	case string:
		return r.pattern.MatchString(value)
	default:
		b, err := sonic.Marshal(value)
		if err != nil {
			return false
		}
		return r.pattern.Match(b)
	}
}

type Simple struct {
	value string
}

func NewSimple(value string) Equality {
	return &Simple{
		value: value,
	}
}

func (s *Simple) Equal(value any) bool {
	switch value := value.(type) {
	case string:
		return s.value == value
	case int:
		return s.value == strconv.Itoa(value)
	case float64:
		return s.value == strconv.FormatFloat(value, 'f', -1, 64)
	default:
		b, err := sonic.Marshal(value)
		if err != nil {
			return false
		}
		return s.value == string(b)
	}
}
