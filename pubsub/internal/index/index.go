// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"context"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
)

const (
	// PublisherEventStoreIndex is the name of the index field for mapping Publishers to their EventStore.
	PublisherEventStoreIndex = "publisherEventStoreIndex"
)

func RegisterIndicesOrDie(ctx context.Context, mgr ctrl.Manager) {
	indexPublisherByEventStore := func(obj client.Object) []string {
		ec, ok := obj.(*pubsubv1.Publisher)
		if !ok {
			return nil
		}
		return []string{ec.Spec.EventStore.Name}
	}
	err := mgr.GetFieldIndexer().IndexField(ctx, &pubsubv1.Publisher{}, PublisherEventStoreIndex, indexPublisherByEventStore)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for EventConfig", "FieldIndex", PublisherEventStoreIndex)
		os.Exit(1)
	}
}
