// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rendering

import (
	sprig "github.com/go-task/slim-sprig/v3"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"testing"
	texttemplate "text/template"
)

func TestRenderer_CustomFunc(t *testing.T) {

	// Create FuncMap with sprig functions
	funcs := sprig.FuncMap()
	funcs["greeter"] = func(name string) string { return "Hello " + name }

	// template to render
	tpl := `This is a test template with a custom function {{ greeter .Name }}.`
	template, _ := texttemplate.New("test").Funcs(funcs).Parse(tpl)

	// do the rendering
	message, err := RenderMessage(template, runtime.RawExtension{
		Raw: []byte(`{"Name":"John"}`)})

	// validate
	assert.NoError(t, err)
	assert.Equal(t, "This is a test template with a custom function Hello John.", message)
}
