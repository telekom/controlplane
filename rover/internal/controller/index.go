// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"os"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller/index"

	ctrl "sigs.k8s.io/controller-runtime"
)

func RegisterIndicesOrDie(ctx context.Context, mgr ctrl.Manager) {

	err := index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &apiapi.Api{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create ownerIndex Api")
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &apiapi.ApiSubscription{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create ownerIndex for ApiSubscription")
		os.Exit(1)
	}
	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &apiapi.ApiExposure{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create ownerIndex for ApiExposure")
		os.Exit(1)
	}
	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &applicationv1.Application{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create ownerIndex for Application")
		os.Exit(1)
	}

}
