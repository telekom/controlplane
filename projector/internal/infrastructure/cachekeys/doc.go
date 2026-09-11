// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package cachekeys provides the single source of truth for EdgeCache key
// construction. Both domain repositories (write path) and IDResolver (read
// path) import these functions, eliminating duplicated key formats and
// preventing silent cache drift.
//
// Every function returns (entityType, lookupKey) suitable for passing directly
// to EdgeCache.Get / Set / Del.
package cachekeys
