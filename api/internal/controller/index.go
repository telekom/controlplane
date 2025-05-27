// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"os"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"
)

func RegisterIndecesOrDie(ctx context.Context, mgr ctrl.Manager) {
	filterStatusActiveOnApi := func(obj client.Object) []string {
		api, ok := obj.(*apiapi.Api)
		if !ok {
			return nil
		}
		if api.Status.Active {
			return []string{"true"}
		}
		return []string{"false"}
	}
	err := mgr.GetFieldIndexer().IndexField(ctx, &apiapi.Api{}, "status.active", filterStatusActiveOnApi)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for Api", "FieldIndex", "status.active")
		os.Exit(1)
	}

	filterStatusActiveOnApiExposure := func(obj client.Object) []string {
		apiExposure, ok := obj.(*apiapi.ApiExposure)
		if !ok {
			return nil
		}
		if apiExposure.Status.Active {
			return []string{"true"}
		}
		return []string{"false"}
	}
	err = mgr.GetFieldIndexer().
		IndexField(ctx, &apiapi.ApiExposure{}, "status.active", filterStatusActiveOnApiExposure)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for ApiExposure", "FieldIndex", "status.active")
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &apiapi.RemoteApiSubscription{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &apiapi.ApiSubscription{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &applicationapi.Application{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &gatewayapi.ConsumeRoute{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &gatewayapi.Route{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &approvalapi.ApprovalRequest{})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field-indexer")
		os.Exit(1)
	}

}
