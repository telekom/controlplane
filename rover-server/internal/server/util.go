// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import "fmt"

func buildCursorUrl(baseURL string, path string, cursor string) string {
	return fmt.Sprintf("%s?cursor=%s", baseURL+path, cursor)
}
