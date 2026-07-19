// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"sort"
	"strings"

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

// FindActiveAgenticServer finds the active AgenticServer for a given basePath.
// Returns (found, mcpServer, error).
// If found is false and mcpServer is non-nil, a server exists but with a different case
// (e.g. /MyMcp vs /mymcp) — the caller should treat this as a conflict, not a missing server.
func FindActiveAgenticServer(ctx context.Context, basePath string) (bool, *agenticv1.AgenticServer, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	serverList := &agenticv1.AgenticServerList{}
	if err := c.List(ctx, serverList, client.MatchingLabels{
		agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(basePath),
	}); err != nil {
		return false, nil, errors.Wrapf(err, "failed to list AgenticServers for basePath %q", basePath)
	}

	// Filter to exact basePath match AND active; also detect case-only mismatches.
	var candidates []agenticv1.AgenticServer
	var caseConflict *agenticv1.AgenticServer
	for i := range serverList.Items {
		server := &serverList.Items[i]
		if !server.Status.Active {
			continue
		}
		if server.Spec.BasePath == basePath {
			candidates = append(candidates, *server)
		} else if strings.EqualFold(server.Spec.BasePath, basePath) && caseConflict == nil {
			caseConflict = server
		}
	}

	if len(candidates) == 0 {
		// Return the case-conflict server (if any) so callers can surface a helpful error.
		return false, caseConflict, nil
	}

	// Sort by CreationTimestamp ascending and return the oldest active one
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreationTimestamp.Before(&candidates[j].CreationTimestamp)
	})

	activeServer := &candidates[0]
	if err := condition.EnsureReady(activeServer); err != nil {
		return false, activeServer, ctrlerrors.BlockedErrorf("AgenticServer %q is not ready", basePath)
	}

	return true, activeServer, nil
}

// FindAgenticExposures lists all AgenticExposures for a given basePath, regardless of status.
func FindAgenticExposures(ctx context.Context, basePath string) ([]agenticv1.AgenticExposure, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	exposureList := &agenticv1.AgenticExposureList{}
	if err := c.List(ctx, exposureList, client.MatchingLabels{
		agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(basePath),
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to list AgenticExposures for basePath %q", basePath)
	}

	var exposures []agenticv1.AgenticExposure
	for i := range exposureList.Items {
		if exposureList.Items[i].Spec.BasePath == basePath {
			exposures = append(exposures, exposureList.Items[i])
		}
	}

	return exposures, nil
}

// FindActiveAgenticExposure finds the active AgenticExposure among a list of exposures.
// Uses oldest-wins semantics based on CreationTimestamp.
func FindActiveAgenticExposure(exposures []agenticv1.AgenticExposure) (bool, *agenticv1.AgenticExposure, error) {
	if len(exposures) == 0 {
		return false, nil, nil
	}

	var candidates []agenticv1.AgenticExposure
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

// AnyOtherAgenticExposureExists checks if any other AgenticExposure exists for the given basePath,
// excluding the one with the given UID.
func AnyOtherAgenticExposureExists(ctx context.Context, basePath string, excludeUID k8stypes.UID) (bool, error) {
	candidates, err := FindAgenticExposures(ctx, basePath)
	if err != nil {
		return false, err
	}

	for i := range candidates {
		if candidates[i].UID == excludeUID {
			continue
		}
		if candidates[i].Spec.BasePath == basePath {
			return true, nil
		}
	}

	return false, nil
}

// FindCrossZoneAgenticSubscriptionZones lists all AgenticSubscriptions for a given basePath
// and returns unique zone ObjectRefs where approved cross-zone subscriptions exist,
// plus a boolean indicating whether any approved same-zone subscriptions also exist.
func FindCrossZoneAgenticSubscriptionZones(ctx context.Context, basePath, exposureZoneName string) (zones []ctypes.ObjectRef, hasLocalSubs bool, err error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	subList := &agenticv1.AgenticSubscriptionList{}
	if err := c.List(ctx, subList, client.MatchingLabels{
		agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(basePath),
	}); err != nil {
		return nil, false, errors.Wrapf(err, "failed to list AgenticSubscriptions for basePath %q", basePath)
	}

	seen := make(map[string]bool)
	for i := range subList.Items {
		sub := &subList.Items[i]
		// Skip subscriptions being deleted
		if controller.IsBeingDeleted(sub) {
			continue
		}
		if sub.Spec.BasePath != basePath {
			continue
		}

		// Only consider approved subscriptions
		approvalCond := meta.FindStatusCondition(sub.GetConditions(), "ApprovalGranted")
		if approvalCond == nil || approvalCond.Status != metav1.ConditionTrue {
			logger.Info("Skipping MCP subscription with missing approval", "subscription", sub.Name, "zone", sub.Spec.Zone.Name)
			continue
		}

		if sub.Spec.Zone.Name == exposureZoneName {
			hasLocalSubs = true
			continue
		}

		zoneName := sub.Spec.Zone.Name
		if !seen[zoneName] {
			seen[zoneName] = true
			zones = append(zones, sub.Spec.Zone)
		}
	}

	return zones, hasLocalSubs, nil
}
