// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package spectre

import (
	"context"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// resolveApplication finds the unique Application by name within the current environment.
func resolveApplication(ctx context.Context, c client.JanitorClient, name string) (*applicationv1.Application, error) {
	list := &applicationv1.ApplicationList{}
	err := c.List(ctx, list, crclient.MatchingLabels{
		config.BuildLabelKey("application"): labelutil.NormalizeValue(name),
	})
	if err != nil {
		return nil, err
	}

	var matched []*applicationv1.Application
	for i := range list.Items {
		app := &list.Items[i]
		if app.Name == name && !controller.IsBeingDeleted(app) {
			matched = append(matched, app)
		}
	}

	switch len(matched) {
	case 0:
		return nil, ctrlerrors.BlockedErrorf("application %q not found", name)
	case 1:
		return matched[0], nil
	default:
		return nil, ctrlerrors.BlockedErrorf("ambiguous: found %d applications named %q", len(matched), name)
	}
}
