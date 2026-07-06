// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

func RegisterIndexesOrDie(ctx context.Context, mgr ctrl.Manager) {
	filterSFTPServiceConfigOnInstance := func(obj client.Object) []string {
		instance, ok := obj.(*sftpv1.Instance)
		if !ok || instance.Spec.SFTPServiceConfigRef.IsEmpty() {
			return nil
		}
		return []string{instance.Spec.SFTPServiceConfigRef.String()}
	}

	err := mgr.GetFieldIndexer().IndexField(ctx, &sftpv1.Instance{}, sftpv1.IndexFieldSpecSFTPServiceConfigRef, filterSFTPServiceConfigOnInstance)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for Instance", "FieldIndex", sftpv1.IndexFieldSpecSFTPServiceConfigRef)
		os.Exit(1)
	}

	filterInstanceOnUser := func(obj client.Object) []string {
		user, ok := obj.(*sftpv1.User)
		if !ok || user.Spec.InstanceRef.IsEmpty() {
			return nil
		}
		return []string{user.Spec.InstanceRef.String()}
	}

	err = mgr.GetFieldIndexer().IndexField(ctx, &sftpv1.User{}, sftpv1.IndexFieldSpecInstanceRef, filterInstanceOnUser)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for User", "FieldIndex", sftpv1.IndexFieldSpecInstanceRef)
		os.Exit(1)
	}
}
