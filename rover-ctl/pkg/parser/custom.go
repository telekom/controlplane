// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser

import "github.com/telekom/controlplane/rover-ctl/pkg/types"

var Opts = []Option{
	WithHook(HookAfterParse, func(obj types.Object) error {
		_, isOpenapiSpec := obj.GetContent()["openapi"]
		if isOpenapiSpec {
			obj.SetKind("ApiSpecification")
			obj.SetApiVersion("tcp.ei.telekom.de/v1")
			obj.SetName("tbd")
		}

		obj.SetProperty("name", obj.GetName())
		obj.SetProperty("kind", obj.GetKind())
		obj.SetProperty("apiVersion", obj.GetApiVersion())

		return nil
	}),
}
