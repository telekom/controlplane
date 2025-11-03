// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//go:build tools
// +build tools

package tools

//go:generate go run github.com/vektra/mockery/v2 --config=mockery.client.yaml
