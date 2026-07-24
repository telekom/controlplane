// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tools

//go:generate go tool oapi-codegen -config service.yaml ../api/service/api.yaml

//go:generate go tool mockery --config=mockery.yaml
