// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"os"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	roverindex "github.com/telekom/controlplane/rover/internal/index"
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

	err = mgr.GetFieldIndexer().IndexField(ctx, &apiapi.ApiCategory{}, roverindex.FieldApiCategoryLabelValue, func(obj client.Object) []string {
		cat, ok := obj.(*apiapi.ApiCategory)
		if !ok {
			ctrl.Log.Error(fmt.Errorf("expected *apiapi.ApiCategory, got %T", obj), "unable to index ApiCategory")
			return nil
		}
		if cat.Spec.LabelValue == "" {
			return nil
		}
		return []string{strings.ToLower(cat.Spec.LabelValue)}
	})
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for ApiCategory", "field", roverindex.FieldApiCategoryLabelValue)
		os.Exit(1)
	}

	if cconfig.FeaturePubSub.IsEnabled() {
		err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &eventv1.EventExposure{})
		if err != nil {
			ctrl.Log.Error(err, "unable to create ownerIndex for EventExposure")
			os.Exit(1)
		}
		err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &eventv1.EventSubscription{})
		if err != nil {
			ctrl.Log.Error(err, "unable to create ownerIndex for EventSubscription")
			os.Exit(1)
		}
		err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &eventv1.EventType{})
		if err != nil {
			ctrl.Log.Error(err, "unable to create ownerIndex for EventType")
			os.Exit(1)
		}
	}

	if cconfig.FeaturePermission.IsEnabled() {
		err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &permissionv1.PermissionSet{})
		if err != nil {
			ctrl.Log.Error(err, "unable to create ownerIndex for PermissionSet")
			os.Exit(1)
		}
	}

	if cconfig.FeatureAiGateway.IsEnabled() {
		err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &agenticv1.McpExposure{})
		if err != nil {
			ctrl.Log.Error(err, "unable to create ownerIndex for McpExposure")
			os.Exit(1)
		}
		err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &agenticv1.McpSubscription{})
		if err != nil {
			ctrl.Log.Error(err, "unable to create ownerIndex for McpSubscription")
			os.Exit(1)
		}
	}
}
