package security

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/pkg/errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	expectedKey  = "expected"
	userInputKey = "userInput"
)

type MatcherFunc func(string, string) bool

type Matcher struct {
	ExpectedTemplate  string
	UserInputTemplate string

	templates map[string]*template.Template
}

func NewMatcher(expectedTemplate, userInputTemplate string) *Matcher {

	funcs := map[string]any{
		"contains": strings.Contains,
		"split":    strings.Split,
		"lastPart": func(s, sep string) string {
			parts := strings.Split(s, sep)
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
			return ""
		},
	}

	return &Matcher{
		ExpectedTemplate:  expectedTemplate,
		UserInputTemplate: userInputTemplate,
		templates: map[string]*template.Template{
			expectedKey:  template.Must(template.New(expectedKey).Funcs(funcs).Parse(expectedTemplate)),
			userInputKey: template.Must(template.New(userInputKey).Funcs(funcs).Parse(userInputTemplate)),
		},
	}
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func (m *Matcher) compileTemplate(name string, compareCtxInfo CompareCtxInfo) (string, error) {
	buf := bufferPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		bufferPool.Put(buf)
	}()

	err := m.templates[name].ExecuteTemplate(buf, name, compareCtxInfo)
	if err != nil {
		return "", errors.Wrapf(err, "failed to execute template: %s", name)
	}
	return buf.String(), nil
}

func (m *Matcher) matches(expected, actual string, matcherFunc MatcherFunc) (bool, error) {
	if len(expected) == 0 {
		return false, errors.New("expected is empty")
	}
	if len(actual) == 0 {
		return false, errors.New("actual is empty")
	}

	return matcherFunc(actual, expected), nil
}

func (m *Matcher) StartsWith(compareCtxInfo CompareCtxInfo) (bool, error) {
	expected, actual, err := m.ToString(compareCtxInfo)
	if err != nil {
		return false, err
	}
	return m.matches(expected, actual, strings.HasPrefix)
}

func (m *Matcher) FullMatch(compareCtxInfo CompareCtxInfo) (bool, error) {
	expected, actual, err := m.ToString(compareCtxInfo)
	if err != nil {
		return false, err
	}
	return m.matches(actual, expected, strings.EqualFold)
}

func (m *Matcher) ToString(compareCtxInfo CompareCtxInfo) (string, string, error) {
	expected, err := m.compileTemplate(expectedKey, compareCtxInfo)
	if err != nil {
		return "", "", err
	}
	userInput, err := m.compileTemplate(userInputKey, compareCtxInfo)
	if err != nil {
		return "", "", err
	}
	return expected, userInput, nil
}

func (m *Matcher) ToExpectedString(bCtx *BusinessContext, params map[string]string) (string, error) {
	expected, err := m.compileTemplate(expectedKey, NewCompareCtxInfo(bCtx, params))
	if err != nil {
		return "", err
	}
	return expected, err
}

type CompareCtxInfo struct {
	B *BusinessContext
	P map[string]string
}

func NewCompareCtxInfo(bCtx *BusinessContext, pathParams map[string]string) CompareCtxInfo {
	toTitle := cases.Title(language.English)
	c := CompareCtxInfo{
		P: make(map[string]string, len(pathParams)),
		B: bCtx,
	}

	for k, v := range pathParams {
		c.P[toTitle.String(k)] = v
	}
	return c
}

type ComparisonTemplates struct {
	ExpectedTemplate  string
	UserInputTemplate string
	MatchType         MatchType
}

func (c ComparisonTemplates) String() string {
	return fmt.Sprintf("Expected %s, UserInput: %s, MatchType %s", c.ExpectedTemplate, c.UserInputTemplate, c.MatchType)
}
