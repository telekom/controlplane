// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"strings"
	"sync"

	"github.com/pkg/errors"
	v0 "github.com/telekom/controlplane/rover-ctl/pkg/handlers/v0"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
)

// handlerRegistry stores the mapping between resource kinds and their handlers
var (
	registry = make(map[string]ResourceHandler)
	mutex    sync.RWMutex

	ErrNoHandlerFound = errors.New("no handler found for the specified resource")
)

// RegisterHandler registers a handler for a specific resource kind and API version
func RegisterHandler(kind, apiVersion string, handler ResourceHandler) {
	mutex.Lock()
	defer mutex.Unlock()

	key := handlerKey(kind, apiVersion)
	registry[key] = handler
	log.L().V(1).Info("Registered handler", "kind", kind, "apiVersion", apiVersion, "key", key)
}

// GetHandler returns the appropriate handler for a resource kind and API version
func GetHandler(kind, apiVersion string) (ResourceHandler, error) {
	mutex.RLock()
	defer mutex.RUnlock()

	key := handlerKey(kind, apiVersion)
	handler, exists := registry[key]
	if !exists {
		return nil, ErrNoHandlerFound
	}

	return handler, nil
}

// handlerKey creates a unique key for the handler registry
func handlerKey(kind, apiVersion string) string {
	return strings.ToLower(apiVersion) + "/" + strings.ToLower(kind)
}

func RegisterHandlers() {
	apiSpecHandler := v0.NewApiSpecHandlerInstance()
	roverHandler := v0.NewRoverHandlerInstance()

	RegisterHandler(apiSpecHandler.Kind, apiSpecHandler.APIVersion, apiSpecHandler)
	RegisterHandler(roverHandler.Kind, roverHandler.APIVersion, roverHandler)
}
