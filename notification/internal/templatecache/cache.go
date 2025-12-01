// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package templatecache

import (
	"sync"
	"text/template"
)

type TemplateWrapper struct {
	BodyTemplate    *template.Template
	SubjectTemplate *template.Template
}

type TemplateCache struct {
	mu    sync.RWMutex
	items map[string]*TemplateWrapper
}

func New() *TemplateCache {
	return &TemplateCache{
		items: make(map[string]*TemplateWrapper),
	}
}

func (c *TemplateCache) Get(name string) (*TemplateWrapper, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	t, ok := c.items[name]
	return t, ok
}

func (c *TemplateCache) Set(name string, tmpl *TemplateWrapper) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[name] = tmpl
}

func (c *TemplateCache) Delete(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, name)
}
