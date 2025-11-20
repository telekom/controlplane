// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	sprig "github.com/go-task/slim-sprig/v3"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"testing"
)

func TestRenderer_CustomFunc(t *testing.T) {

	// Create FuncMap with sprig functions
	funcs := sprig.FuncMap()
	funcs["greeter"] = func(name string) string { return "Hello " + name }

	// create new renderer with a custom function
	r := NewRenderer(funcs)

	// template to render
	tpl := `This is a test template with a custom function {{ greeter .Name }}.`

	// do the rendering
	message, err := r.renderMessage(tpl, runtime.RawExtension{
		Raw: []byte(`{"Name":"John"}`)})

	// validate
	assert.NoError(t, err)
	assert.Equal(t, "This is a test template with a custom function Hello John.", message)
}
