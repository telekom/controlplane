// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
)

const IndexFieldRealmName = ".spec.realmName"

// RegisterIndexesOrDie registers field indexes required by the webhook validators.
func RegisterIndexesOrDie(ctx context.Context, mgr ctrl.Manager) {
	err := mgr.GetFieldIndexer().IndexField(ctx, &adminv1.Environment{}, IndexFieldRealmName, func(obj client.Object) []string {
		env, ok := obj.(*adminv1.Environment)
		if !ok {
			return nil
		}
		return []string{env.GetRealmName()}
	})
	if err != nil {
		ctrl.Log.Error(err, "unable to create field index for Environment", "field", IndexFieldRealmName)
		panic(err)
	}
}
