// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"context"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	filev1 "github.com/telekom/controlplane/file/api/v1"
)

const (
	// FieldSpecFileTypeOnExposure indexes FileExposures by their spec.fileType field.
	FieldSpecFileTypeOnExposure = "spec.fileType.exposure"
	// FieldSpecZoneOnExposure indexes FileExposures by their spec.zone (namespace/name).
	FieldSpecZoneOnExposure = "spec.zone.exposure"
	// FieldSpecFileTypeOnSubscription indexes FileSubscriptions by their spec.fileType field.
	FieldSpecFileTypeOnSubscription = "spec.fileType.subscription"
)

func RegisterIndicesOrDie(ctx context.Context, mgr ctrl.Manager) {
	if err := index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &approvalv1.ApprovalRequest{}); err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}

	if err := mgr.GetFieldIndexer().IndexField(ctx, &filev1.FileExposure{}, FieldSpecFileTypeOnExposure, func(obj client.Object) []string {
		exposure, ok := obj.(*filev1.FileExposure)
		if !ok {
			return nil
		}
		return []string{exposure.Spec.FileType}
	}); err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for FileExposure", "field", FieldSpecFileTypeOnExposure)
		os.Exit(1)
	}

	if err := mgr.GetFieldIndexer().IndexField(ctx, &filev1.FileExposure{}, FieldSpecZoneOnExposure, func(obj client.Object) []string {
		exposure, ok := obj.(*filev1.FileExposure)
		if !ok || exposure.Spec.Zone == nil {
			return nil
		}
		return []string{exposure.Spec.Zone.String()}
	}); err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for FileExposure", "field", FieldSpecZoneOnExposure)
		os.Exit(1)
	}

	if err := mgr.GetFieldIndexer().IndexField(ctx, &filev1.FileSubscription{}, FieldSpecFileTypeOnSubscription, func(obj client.Object) []string {
		subscription, ok := obj.(*filev1.FileSubscription)
		if !ok {
			return nil
		}
		return []string{subscription.Spec.FileType}
	}); err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for FileSubscription", "field", FieldSpecFileTypeOnSubscription)
		os.Exit(1)
	}
}
