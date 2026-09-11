// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tools

//go:generate go tool oapi-codegen --config=oapi-codegen-model-config.yaml ../pkg/api/openapi.yaml
//go:generate go tool oapi-codegen --config=oapi-codegen-client-config.yaml ../pkg/api/openapi.yaml
//go:generate go tool mockery --config=mockery.yaml
