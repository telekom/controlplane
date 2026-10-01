// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package spectre

import (
	"context"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
)

// applicationKeyFromID returns the object key of the Application addressed by a
// full application ID "<group>--<team>--<name>". ok is false for any other format.
func applicationKeyFromID(ctx context.Context, ref string) (key crclient.ObjectKey, ok bool) {
	parts := strings.SplitN(ref, "--", 3)
	if len(parts) != 3 {
		return crclient.ObjectKey{}, false
	}
	return crclient.ObjectKey{
		Name:      parts[2],
		Namespace: organizationv1.TeamNamespace(contextutil.EnvFromContextOrDie(ctx), parts[0]+"--"+parts[1]),
	}, true
}

// resolveApplication finds the Application referenced by ref within the current environment.
// A full application ID "<group>--<team>--<name>" is looked up in that team's namespace;
// any other value is treated as a bare name that must be unique across all teams.
func resolveApplication(ctx context.Context, c client.JanitorClient, ref string) (*applicationv1.Application, error) {
	if key, ok := applicationKeyFromID(ctx, ref); ok {
		app := &applicationv1.Application{}
		err := c.Get(ctx, key, app)
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		if err != nil || controller.IsBeingDeleted(app) {
			return nil, ctrlerrors.BlockedErrorf("application %q not found in team namespace %q", key.Name, key.Namespace)
		}
		return app, nil
	}

	list := &applicationv1.ApplicationList{}
	err := c.List(ctx, list, crclient.MatchingLabels{
		config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(ref),
	})
	if err != nil {
		return nil, err
	}

	var matched []*applicationv1.Application
	for i := range list.Items {
		app := &list.Items[i]
		if app.Name == ref && !controller.IsBeingDeleted(app) {
			matched = append(matched, app)
		}
	}

	switch len(matched) {
	case 0:
		return nil, ctrlerrors.BlockedErrorf("application %q not found", ref)
	case 1:
		return matched[0], nil
	default:
		return nil, ctrlerrors.BlockedErrorf("ambiguous: found %d applications named %q", len(matched), ref)
	}
}
