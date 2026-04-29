// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//go:build tools
// +build tools

package tools

//go:generate go tool oapi-codegen --config=oapi-codegen-client-config.yaml ../pkg/kong/api/openapi.yaml

// contains mocks for the KongClient and the KongAdminApi
// WARN - KongClient was generated previously - code needs to be adjusted before using the mockery version
//go:generate go tool mockery --config=mockery.yaml
