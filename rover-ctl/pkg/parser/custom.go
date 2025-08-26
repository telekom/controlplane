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

		obj.SetProperty("name", obj.GetName())
		obj.SetProperty("kind", obj.GetKind())
		obj.SetProperty("apiVersion", obj.GetApiVersion())

		return nil
	}),
}
