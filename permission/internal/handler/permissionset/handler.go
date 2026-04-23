// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package permissionset

import (
	"context"

	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	pcpv1 "github.com/telekom/controlplane/permission/api/pcp/v1"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// LegacyEnvironmentLabelKey is the label used by the external permission service
	// to identify the environment. This is separate from the new system label.
	LegacyEnvironmentLabelKey = "ei.telekom.de/environment"
)

var _ handler.Handler[*permissionv1.PermissionSet] = (*PermissionSetHandler)(nil)

type PermissionSetHandler struct{}

func (h *PermissionSetHandler) CreateOrUpdate(ctx context.Context, obj *permissionv1.PermissionSet) error {
	log := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)
	environment := contextutil.EnvFromContextOrDie(ctx)

	// Note: external PermissionSet (pcpv1) is not tracked via AddKnownTypeToState
	// because it doesn't implement types.Object. Cross-namespace cleanup uses
	// label-based cleanup via OwnedByLabel instead.

	// Get the Zone to determine the target namespace for the external PermissionSet
	// The zone name comes from the owner (Rover), which should be available via labels
	zoneName := obj.Labels[config.BuildLabelKey("zone")]
	if zoneName == "" {
		obj.SetCondition(condition.NewNotReadyCondition("MissingZone", "Zone label is missing"))
		return errors.New("zone label is required but not found")
	}

	zone := &adminv1.Zone{}
	zoneKey := client.ObjectKey{
		Name:      zoneName,
		Namespace: environment,
	}
	if err := c.Get(ctx, zoneKey, zone); err != nil {
		obj.SetCondition(condition.NewNotReadyCondition("ZoneNotFound", "Failed to lookup zone"))
		return errors.Wrapf(err, "failed to get zone %s", zoneName)
	}

	if err := condition.EnsureReady(zone); err != nil {
		obj.SetCondition(condition.NewBlockedCondition("Zone is not ready yet"))
		return nil
	}

	// Create external PermissionSet in the zone namespace
	// Use namespace-prefixed name to avoid collisions from different namespaces
	externalName := labelutil.NormalizeNameValue(obj.Namespace + "-" + obj.Name)
	externalPS := &pcpv1.PermissionSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalName,
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		// Cross-namespace: use owner.uid label for cleanup tracking
		if externalPS.Labels == nil {
			externalPS.Labels = make(map[string]string)
		}
		externalPS.Labels[config.OwnerUidLabelKey] = string(obj.GetUID())

		// Set both environment labels for compatibility
		// Legacy label for external service compatibility
		externalPS.Labels[LegacyEnvironmentLabelKey] = environment

		// Copy permissions directly - spec structures are identical
		externalPS.Spec.Permissions = make([]pcpv1.Permission, len(obj.Spec.Permissions))
		for i, perm := range obj.Spec.Permissions {
			externalPS.Spec.Permissions[i] = pcpv1.Permission{
				Role:     perm.Role,
				Resource: perm.Resource,
				Actions:  perm.Actions,
			}
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, externalPS, mutator)
	if err != nil {
		obj.SetCondition(condition.NewNotReadyCondition("ProvisioningFailed", "Failed to create external PermissionSet"))
		return errors.Wrap(err, "failed to create or update external PermissionSet")
	}

	// Store reference to external PermissionSet
	obj.Status.PermissionSet = types.ObjectRefFromObject(externalPS)

	// External PermissionSet has no status/conditions, so it's immediately "ready" after creation
	// Set our internal PermissionSet to ready
	obj.SetCondition(condition.NewReadyCondition("Provisioned", "External PermissionSet created successfully"))
	obj.SetCondition(condition.NewDoneProcessingCondition("External PermissionSet provisioned"))

	log.Info("PermissionSet provisioned", "external", externalPS.Namespace+"/"+externalPS.Name)

	// Cleanup orphaned external PermissionSets
	// Use OwnedByLabel since external PS is in a different namespace
	if _, err := c.CleanupAll(ctx, cclient.OwnedByLabel(obj)); err != nil {
		return errors.Wrap(err, "failed to cleanup orphaned external PermissionSets")
	}

	return nil
}

func (h *PermissionSetHandler) Delete(ctx context.Context, obj *permissionv1.PermissionSet) error {
	log := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	// If we have a status reference to the external PermissionSet, delete it
	if obj.Status.PermissionSet != nil {
		externalPS := &pcpv1.PermissionSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      obj.Status.PermissionSet.Name,
				Namespace: obj.Status.PermissionSet.Namespace,
			},
		}

		if err := c.Delete(ctx, externalPS); client.IgnoreNotFound(err) != nil {
			return errors.Wrap(err, "failed to delete external PermissionSet")
		} else if err == nil {
			log.Info("Deleted external PermissionSet", "name", externalPS.Name, "namespace", externalPS.Namespace)
		} else {
			log.V(1).Info("External PermissionSet not found (already deleted)", "name", externalPS.Name, "namespace", externalPS.Namespace)
		}
	}

	return nil
}
