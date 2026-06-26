// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"context"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller/index"
)

func RegisterIndicesOrDie(ctx context.Context, mgr ctrl.Manager) {
	if err := index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &approvalv1.ApprovalRequest{}); err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}
}
