// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package apiroadmap projects ApiRoadmap CRs onto the roadmap file ID of
// the matching Api catalogue entity. It does not own a table of its own.
package apiroadmap

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// Key identifies the Api (base path and owning team) a roadmap belongs to.
// FileID lets a delete find a link that was left on a previous base path by a retarget.
type Key struct {
	BasePath  string
	Namespace string
	TeamName  string
	FileID    string
}

// Data carries the file-manager file ID of an API roadmap.
type Data struct {
	Meta      shared.Metadata
	BasePath  string
	Namespace string
	TeamName  string
	FileID    string
}
