// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package apichangelog projects ApiChangelog CRs onto the changelog file ID of
// the matching Api catalogue entity. It does not own a table of its own.
package apichangelog

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// Key identifies the Api (base path and owning team) a changelog belongs to.
// FileID lets a delete find a link that was left on a previous base path by a retarget.
type Key struct {
	BasePath  string
	Namespace string
	TeamName  string
	FileID    string
}

// Data carries the file-manager file ID of an API changelog.
type Data struct {
	Meta      shared.Metadata
	BasePath  string
	Namespace string
	TeamName  string
	FileID    string
}
