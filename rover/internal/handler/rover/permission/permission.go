// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package permission

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// HandlePermission creates or updates PermissionSet resources based on Rover permission configuration
func HandlePermission(ctx context.Context, c client.JanitorClient, owner *roverv1.Rover) error {
	// Normalize all permission formats into flat permissions
	permissions := normalizePermissions(owner.Spec.Permissions)

	// Create internal PermissionSet
	ps := &permissionv1.PermissionSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(owner.Name),
			Namespace: owner.Namespace,
		},
	}

	mutator := func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(owner, ps, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		// Set labels
		if ps.Labels == nil {
			ps.Labels = make(map[string]string)
		}
		ps.Labels[config.BuildLabelKey("application")] = labelutil.NormalizeValue(owner.Name)
		if owner.Spec.Zone != "" {
			ps.Labels[config.BuildLabelKey("zone")] = labelutil.NormalizeValue(owner.Spec.Zone)
		}

		// Set permissions spec
		ps.Spec.Permissions = permissions

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, ps, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update PermissionSet")
	}

	// Append to status
	owner.Status.PermissionSets = append(owner.Status.PermissionSets, *types.ObjectRefFromObject(ps))

	return nil
}

// normalizePermissions converts all 3 permission formats into a flat list of permissions
// Formats supported:
// 1. Resource-oriented: resource + entries (role/actions list)
// 2. Role-oriented: role + entries (resource/actions list)
// 3. Flat: role + resource + actions directly
func normalizePermissions(perms []roverv1.Permission) []permissionv1.Permission {
	var permissions []permissionv1.Permission

	for _, p := range perms {
		if p.Resource != "" && p.Role == "" && len(p.Entries) > 0 {
			// Resource-oriented format: resource + entries (with roles)
			for _, entry := range p.Entries {
				permissions = append(permissions, permissionv1.Permission{
					Resource: p.Resource,
					Role:     entry.Role,
					Actions:  entry.Actions,
				})
			}
		} else if p.Role != "" && p.Resource == "" && len(p.Entries) > 0 {
			// Role-oriented format: role + entries (with resources)
			for _, entry := range p.Entries {
				permissions = append(permissions, permissionv1.Permission{
					Role:     p.Role,
					Resource: entry.Resource,
					Actions:  entry.Actions,
				})
			}
		} else if p.Resource != "" && p.Role != "" && len(p.Actions) > 0 {
			// Flat format: role + resource + actions directly
			permissions = append(permissions, permissionv1.Permission{
				Role:     p.Role,
				Resource: p.Resource,
				Actions:  p.Actions,
			})
		}
	}

	return permissions
}
