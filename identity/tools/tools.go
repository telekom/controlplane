// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//go:build tools
// +build tools

package tools

import (
	_ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
	_ "github.com/vektra/mockery/v3"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=oapi-codegen-model-config.yaml ../pkg/api/openapi.yaml
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=oapi-codegen-client-config.yaml ../pkg/api/openapi.yaml
//go:generate go run github.com/vektra/mockery/v3 --config=mockery.yaml
