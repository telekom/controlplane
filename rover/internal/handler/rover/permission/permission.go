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

// HandlePermission creates or updates PermissionSet resources based on Rover authorization configuration
func HandlePermission(ctx context.Context, c client.JanitorClient, owner *roverv1.Rover) error {
	// Normalize all authorization formats into flat permissions
	permissions := normalizeAuthorization(owner.Spec.Authorization)

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

// normalizeAuthorization converts all 3 authorization formats into a flat list of permissions
// Formats supported:
// 1. Resource-oriented: resource + permissions (role/actions list)
// 2. Role-oriented: role + permissions (resource/actions list)
// 3. Flat: role + resource + actions directly
func normalizeAuthorization(authorizations []roverv1.Authorization) []permissionv1.Permission {
	var permissions []permissionv1.Permission

	for _, auth := range authorizations {
		if auth.Resource != "" && auth.Role == "" && len(auth.Permissions) > 0 {
			// Resource-oriented format: resource + permissions (with roles)
			for _, perm := range auth.Permissions {
				permissions = append(permissions, permissionv1.Permission{
					Resource: auth.Resource,
					Role:     perm.Role,
					Actions:  perm.Actions,
				})
			}
		} else if auth.Role != "" && auth.Resource == "" && len(auth.Permissions) > 0 {
			// Role-oriented format: role + permissions (with resources)
			for _, perm := range auth.Permissions {
				permissions = append(permissions, permissionv1.Permission{
					Role:     auth.Role,
					Resource: perm.Resource,
					Actions:  perm.Actions,
				})
			}
		} else if auth.Resource != "" && auth.Role != "" && len(auth.Actions) > 0 {
			// Flat format: role + resource + actions directly
			permissions = append(permissions, permissionv1.Permission{
				Role:     auth.Role,
				Resource: auth.Resource,
				Actions:  auth.Actions,
			})
		}
	}

	return permissions
}
