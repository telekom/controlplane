// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var Opts = []Option{
	WithHook(HookAfterParse, func(obj types.Object) error {
		_, isOpenapiSpec := obj.GetContent()["openapi"]
		_, isSwagger := obj.GetContent()["swagger"]
		if isOpenapiSpec || isSwagger {
			obj.SetProperty("kind", "ApiSpecification")
			obj.SetProperty("apiVersion", "tcp.ei.telekom.de/v1")
			return ParseApiSpecification(obj)
		}

		_, hasBasePath := obj.GetContent()["basePath"]
		_, hasTools := obj.GetContent()["tools"]
		_, hasPrompts := obj.GetContent()["prompts"]
		_, hasResources := obj.GetContent()["resources"]
		if hasBasePath && (hasTools || hasPrompts || hasResources) {
			obj.SetProperty("kind", "McpSpecification")
			obj.SetProperty("apiVersion", "tcp.ei.telekom.de/v1")
			return ParseMcpSpecification(obj)
		}

		obj.SetProperty("name", obj.GetName())
		obj.SetProperty("kind", obj.GetKind())
		obj.SetProperty("apiVersion", obj.GetApiVersion())

		return nil
	}),
}
