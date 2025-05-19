// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func OwnedBy(owner client.Object) []client.ListOption {

	ownerUID := string(owner.GetUID())

	if ownerUID == "" {
		panic("owner UID is nil")
	}

	return []client.ListOption{
		client.MatchingFields{
			index.ControllerIndexKey: ownerUID,
		},
	}
}

// OwnedByLabel should only be used when ownership could not be determined by controllerRef because of cross-namespace references
func OwnedByLabel(owner client.Object) []client.ListOption {

	ownerUID := string(owner.GetUID())

	if ownerUID == "" {
		panic("owner UID is nil")
	}

	return []client.ListOption{
		client.MatchingLabels{
			config.OwnerUidLabelKey: ownerUID,
		},
	}
}

func DoNothing() controllerutil.MutateFn {
	return func() error {
		return nil
	}
}
