// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
// Copyright 2026.
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// UidIndexKey is the field index path for looking up Spectre resources by UID.
const UidIndexKey = ".metadata.uid"

// RegisterUidIndex registers a field index that enables efficient lookup of a
// resource by its metadata.uid. This replaces cluster-wide parent scans in
// owned-child mappers with indexed lookups.
func RegisterUidIndex(ctx context.Context, indexer client.FieldIndexer, obj client.Object) error {
	return indexer.IndexField(ctx, obj, UidIndexKey, func(o client.Object) []string {
		uid := string(o.GetUID())
		if uid == "" {
			return nil
		}
		return []string{uid}
	})
}

// RegisterIndices registers all field indexes required by the Spectre controllers.
func RegisterIndices(ctx context.Context, indexer client.FieldIndexer) error {
	if err := RegisterUidIndex(ctx, indexer, &spectrev1.Listener{}); err != nil {
		return err
	}
	if err := RegisterUidIndex(ctx, indexer, &spectrev1.SpectreApplication{}); err != nil {
		return err
	}
	return nil
}
