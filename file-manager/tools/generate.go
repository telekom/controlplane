// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//go:build tools
// +build tools

package tools

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=server.yaml ../api/openapi.yaml
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=client.yaml ../api/openapi.yaml

// support for testing in domains that want to use the file manager client
//go:generate go run github.com/vektra/mockery/v2 --config=mockery.client.yaml

// for testing the implementation of the client - server
//go:generate go run github.com/vektra/mockery/v2 --config=mockery.api.yaml

// for controller testing
//go:generate go run github.com/vektra/mockery/v2 --config=mockery.server.yaml
