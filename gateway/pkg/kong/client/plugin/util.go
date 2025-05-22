// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

func As[T client.CustomPlugin](s client.CustomPlugin, t *T) bool {
	if st, ok := s.(T); ok {
		*t = st
		return true
	}
	return false
}
