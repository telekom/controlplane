// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

// GetZone retrieves a Zone object by ObjectRef and ensures it is ready.
func GetZone(ctx context.Context, ref client.ObjectKey) (*adminv1.Zone, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	zone := &adminv1.Zone{}
	err := c.Get(ctx, ref, zone)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("zone %q not found", ref.String())
		}
		return nil, errors.Wrapf(err, "failed to get zone %q", ref.String())
	}
	if err := condition.EnsureReady(zone); err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q is not ready", ref.String())
	}

	return zone, nil
}

// GetApplication retrieves an Application object by ObjectRef and ensures it is ready.
func GetApplication(ctx context.Context, ref ctypes.ObjectRef) (*applicationapi.Application, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	application := &applicationapi.Application{}
	err := c.Get(ctx, ref.K8s(), application)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("application %q not found", ref.String())
		}
		return nil, errors.Wrapf(err, "failed to get application %q", ref.String())
	}
	if err := condition.EnsureReady(application); err != nil {
		return nil, ctrlerrors.BlockedErrorf("application %q is not ready", ref.String())
	}

	return application, nil
}

// FindActiveMcpServer finds the active McpServer for a given basePath.
// Returns (found, mcpServer, error). If found is false, there is no active McpServer.
func FindActiveMcpServer(ctx context.Context, basePath string) (bool, *agenticv1.McpServer, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	serverList := &agenticv1.McpServerList{}
	if err := c.List(ctx, serverList, client.MatchingLabels{
		agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(basePath),
	}); err != nil {
		return false, nil, errors.Wrapf(err, "failed to list McpServers for basePath %q", basePath)
	}

	// Filter to matching basePath and active
	var candidates []agenticv1.McpServer
	for i := range serverList.Items {
		if serverList.Items[i].Spec.BasePath == basePath && serverList.Items[i].Status.Active {
			candidates = append(candidates, serverList.Items[i])
		}
	}

	if len(candidates) == 0 {
		return false, nil, nil
	}

	// Sort by CreationTimestamp ascending and return the oldest active one
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreationTimestamp.Before(&candidates[j].CreationTimestamp)
	})

	activeServer := &candidates[0]
	if err := condition.EnsureReady(activeServer); err != nil {
		return false, activeServer, ctrlerrors.BlockedErrorf("McpServer %q is not ready", basePath)
	}

	return true, activeServer, nil
}

// FindMcpExposures lists all McpExposures for a given basePath, regardless of status.
func FindMcpExposures(ctx context.Context, basePath string) ([]agenticv1.McpExposure, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	exposureList := &agenticv1.McpExposureList{}
	if err := c.List(ctx, exposureList, client.MatchingLabels{
		agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(basePath),
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to list McpExposures for basePath %q", basePath)
	}

	var exposures []agenticv1.McpExposure
	for i := range exposureList.Items {
		if exposureList.Items[i].Spec.McpBasePath == basePath {
			exposures = append(exposures, exposureList.Items[i])
		}
	}

	return exposures, nil
}

// FindActiveMcpExposure finds the active McpExposure among a list of exposures.
// Uses oldest-wins semantics based on CreationTimestamp.
func FindActiveMcpExposure(exposures []agenticv1.McpExposure) (bool, *agenticv1.McpExposure, error) {
	if len(exposures) == 0 {
		return false, nil, nil
	}

	var candidates []agenticv1.McpExposure
	for i := range exposures {
		if exposures[i].Status.Active {
			candidates = append(candidates, exposures[i])
		}
	}

	if len(candidates) == 0 {
		return false, nil, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreationTimestamp.Before(&candidates[j].CreationTimestamp)
	})

	activeExp := &candidates[0]
	return true, activeExp, nil
}

// AnyOtherMcpExposureExists checks if any other McpExposure exists for the given basePath,
// excluding the one with the given UID.
func AnyOtherMcpExposureExists(ctx context.Context, basePath string, excludeUID k8stypes.UID) (bool, error) {
	candidates, err := FindMcpExposures(ctx, basePath)
	if err != nil {
		return false, err
	}

	for i := range candidates {
		if candidates[i].UID == excludeUID {
			continue
		}
		if candidates[i].Spec.McpBasePath == basePath {
			return true, nil
		}
	}

	return false, nil
}

// FindCrossZoneMcpSubscriptionZones lists all McpSubscriptions for a given basePath
// and returns unique zone ObjectRefs where approved cross-zone subscriptions exist.
func FindCrossZoneMcpSubscriptionZones(ctx context.Context, basePath, exposureZoneName string) ([]ctypes.ObjectRef, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	subList := &agenticv1.McpSubscriptionList{}
	if err := c.List(ctx, subList, client.MatchingLabels{
		agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(basePath),
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to list McpSubscriptions for basePath %q", basePath)
	}

	seen := make(map[string]bool)
	var zones []ctypes.ObjectRef
	for i := range subList.Items {
		sub := &subList.Items[i]
		// Skip subscriptions being deleted
		if controller.IsBeingDeleted(sub) {
			continue
		}
		if sub.Spec.McpBasePath != basePath {
			continue
		}
		// Skip same-zone subscriptions
		if sub.Spec.Zone.Name == exposureZoneName {
			continue
		}

		// Only include approved subscriptions
		approvalCond := meta.FindStatusCondition(sub.GetConditions(), "ApprovalGranted")
		if approvalCond == nil || approvalCond.Status != metav1.ConditionTrue {
			logger.Info("Skipping MCP subscription with missing approval", "subscription", sub.Name, "zone", sub.Spec.Zone.Name)
			continue
		}

		zoneName := sub.Spec.Zone.Name
		if !seen[zoneName] {
			seen[zoneName] = true
			zones = append(zones, sub.Spec.Zone)
		}
	}

	return zones, nil
}
