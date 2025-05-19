package template

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/telekom/controlplane/common-server/pkg/problems"
)

const placeholderChars = "$<>"

var placeholderPattern = regexp.MustCompile(`^\$\<\w+\>$`)

// Template is used to resolve placeholders in a map[string]any or string value
// It does to by checking in a `lookUp` map for the value of the placeholder
// The placeholder is defined by the pattern `$<placeholder>`
type Template struct {
	Value any
}

func New(value any) Template {
	return Template{Value: value}
}

func (t Template) GetAllPlaceholders() []string {
	return getAllPlaceholders(t.Value)
}

func getAllPlaceholders(value any) []string {
	if value == nil {
		return nil
	}
	if v, ok := value.(string); ok {
		if IsPlaceholder(v) {
			return []string{strings.Trim(v, placeholderChars)}
		}
		return nil
	}
	if v, ok := value.(map[string]any); ok {
		res := []string{}
		for _, v := range v {
			res = append(res, getAllPlaceholders(v)...)
		}
		return res
	}
	return nil
}

func (t Template) IsMap() bool {
	_, ok := t.Value.(map[string]any)
	return ok
}

func (t Template) IsString() bool {
	_, ok := t.Value.(string)
	return ok
}

type TemplateResolver func(lookUp map[string]any, key string) (any, error)

func (t Template) Apply(lookUp map[string]any) (any, error) {
	if t.IsMap() {
		return applyMapTemplate(t.Value.(map[string]any), lookUp)
	}
	if t.IsString() {
		return applyStringTemplate(t.Value.(string), lookUp)
	}
	return nil, problems.BadRequest("invalid template")
}

func applyMapTemplate(m map[string]any, lookUp map[string]any) (map[string]any, error) {
	res := map[string]any{}
	for k, v := range m {
		switch v := v.(type) {
		case string:
			val, err := resolveStringTemplate(v, lookUp)
			if err != nil {
				return nil, err
			}
			res[k] = val
		case map[string]any:
			sub, err := applyMapTemplate(v, lookUp)
			if err != nil {
				return nil, err
			}
			res[k] = sub
		}
	}
	return res, nil
}

func applyStringTemplate(s string, lookUp map[string]any) (any, error) {
	if !IsPlaceholder(s) {
		return s, nil
	}
	key := strings.Trim(s, placeholderChars)
	val, ok := lookUp[key]
	if !ok {
		return nil, problems.BadRequest(fmt.Sprintf("missing value for %s", key))
	}
	return val, nil
}

func resolveStringTemplate(s string, lookUp map[string]any) (any, error) {
	if !IsPlaceholder(s) {
		return s, nil
	}
	key := strings.Trim(s, placeholderChars)
	val, ok := lookUp[key]
	if !ok {
		return nil, problems.BadRequest(fmt.Sprintf("missing value for %s", key))
	}
	return val, nil
}

func IsPlaceholder(s any) bool {
	switch s := s.(type) {
	case string:
		return placeholderPattern.MatchString(s)
	case map[string]any:
		for _, v := range s {
			if IsPlaceholder(v) {
				return true
			}
		}
	}
	return false
}

func Trim(s string) string {
	return strings.Trim(s, placeholderChars)
}
