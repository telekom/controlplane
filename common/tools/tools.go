// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//go:build tools
// +build tools

package tools

import (
	_ "github.com/golang/mock/mockgen"
)

//go:generate go run github.com/golang/mock/mockgen -destination=../pkg/client/mocks/janitor_client.go github.com/telekom/controlplane/common/pkg/client JanitorClient
