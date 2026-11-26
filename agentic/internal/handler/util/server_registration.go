// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"cmp"
	"context"
	"slices"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

// CompareServerRegistrations orders server registrations by creation time, namespace, name, then kind.
func CompareServerRegistrations(a, b client.Object) int {
	if c := a.GetCreationTimestamp().Compare(b.GetCreationTimestamp().Time); c != 0 {
		return c
	}
	if c := cmp.Compare(a.GetNamespace(), b.GetNamespace()); c != 0 {
		return c
	}
	if c := cmp.Compare(a.GetName(), b.GetName()); c != 0 {
		return c
	}
	return cmp.Compare(ServerRegistrationKind(a), ServerRegistrationKind(b))
}

// ServerRegistrationKind returns the kind name used in registration conflict messages.
func ServerRegistrationKind(obj client.Object) string {
	switch obj.(type) {
	case *agenticv1.McpServer:
		return "McpServer"
	case *agenticv1.AgentCard:
		return "AgentCard"
	default:
		return obj.GetObjectKind().GroupVersionKind().Kind
	}
}

// FindOldestPeerRegistration finds the oldest exact-basePath registration of the other kind.
func FindOldestPeerRegistration(ctx context.Context, obj client.Object) (client.Object, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	var basePath string
	switch registration := obj.(type) {
	case *agenticv1.McpServer:
		basePath = registration.Spec.BasePath
	case *agenticv1.AgentCard:
		basePath = registration.Spec.BasePath
	default:
		return nil, errors.Errorf("unsupported server registration kind %T", obj)
	}

	label := client.MatchingLabels{
		cconfig.EnvironmentLabelKey: obj.GetLabels()[cconfig.EnvironmentLabelKey],
		agenticv1.BasePathLabelKey:  labelutil.NormalizeLabelValue(basePath),
	}

	var candidates []client.Object
	switch obj.(type) {
	case *agenticv1.McpServer:
		cardList := &agenticv1.AgentCardList{}
		if err := c.List(ctx, cardList, label); err != nil {
			return nil, errors.Wrapf(err, "failed to list AgentCards for basePath %q", basePath)
		}
		for i := range cardList.Items {
			if cardList.Items[i].Spec.BasePath == basePath {
				candidates = append(candidates, &cardList.Items[i])
			}
		}
	case *agenticv1.AgentCard:
		serverList := &agenticv1.McpServerList{}
		if err := c.List(ctx, serverList, label); err != nil {
			return nil, errors.Wrapf(err, "failed to list McpServers for basePath %q", basePath)
		}
		for i := range serverList.Items {
			if serverList.Items[i].Spec.BasePath == basePath {
				candidates = append(candidates, &serverList.Items[i])
			}
		}
	default:
		return nil, errors.Errorf("unsupported server registration kind %T", obj)
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	slices.SortFunc(candidates, CompareServerRegistrations)
	return candidates[0], nil
}
