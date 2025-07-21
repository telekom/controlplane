// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//go:build tools
// +build tools

package tools

import (
	_ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
	_ "github.com/vektra/mockery/v2"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=server.yaml ../api/file-manager-oas.yaml
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=client.yaml ../api/file-manager-oas.yaml

////go:generate go run github.com/vektra/mockery/v2 --config=mockery.yaml

//go:generate go run github.com/vektra/mockery/v2 --config=mockery.api.yaml
