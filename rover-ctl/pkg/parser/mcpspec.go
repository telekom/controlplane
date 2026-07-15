// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"strings"

	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// ParseMcpSpecification extracts the name from an MCP specification's basePath.
// The name is derived by stripping leading slashes and replacing "/" with "-".
func ParseMcpSpecification(obj types.Object) error {
	content := obj.GetContent()

	basePath, _ := content["basePath"].(string)
	if basePath == "" {
		return nil
	}

	name := strings.TrimPrefix(basePath, "/")
	name = strings.ToLower(strings.ReplaceAll(name, "/", "-"))
	obj.SetProperty("name", name)
	return nil
}
