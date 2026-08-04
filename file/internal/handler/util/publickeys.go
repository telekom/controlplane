// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"slices"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

func SyncSFTPUser(
	ctx context.Context,
	userRef types.ObjectRef,
	owner client.Object,
	fileTypeRef types.ObjectRef,
	publicKeys []filev1.SSHPublicKeySpec,
	instanceRef types.ObjectRef,
) (*sftpv1.User, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	user := &sftpv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userRef.Name,
			Namespace: userRef.Namespace,
		},
	}

	keys, err := CanonicalSSHPublicKeys(publicKeys)
	if err != nil {
		return nil, err
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(owner, user, c.Scheme()); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		user.Labels = ChildLabels(fileTypeRef)
		user.Spec.InstanceRef = instanceRef
		user.Spec.SSHPublicKeys = keys
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, user, mutator); err != nil {
		return nil, fmt.Errorf("failed to sync SFTP User %q: %w", userRef.String(), err)
	}
	return user, nil
}

func DeleteSFTPUser(ctx context.Context, userRef types.ObjectRef) error {
	c := cclient.ClientFromContextOrDie(ctx)
	user := &sftpv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userRef.Name,
			Namespace: userRef.Namespace,
		},
	}

	if err := c.Delete(ctx, user); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil
		}
		return fmt.Errorf("failed to delete SFTP User %q: %w", userRef.String(), err)
	}
	return nil
}

func DeleteSFTPInstance(ctx context.Context, instanceRef types.ObjectRef) error {
	c := cclient.ClientFromContextOrDie(ctx)
	instance := &sftpv1.Instance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceRef.Name,
			Namespace: instanceRef.Namespace,
		},
	}

	if err := c.Delete(ctx, instance); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil
		}
		return fmt.Errorf("failed to delete SFTP Instance %q: %w", instanceRef.String(), err)
	}
	return nil
}

func CanonicalSSHPublicKeys(publicKeys []filev1.SSHPublicKeySpec) ([]string, error) {
	canonicalKeys := make([]string, 0, len(publicKeys))

	for i := range publicKeys {
		canonicalKey, err := sftpv1.CanonicalPublicKey(publicKeys[i].Key)
		if err != nil {
			return nil, fmt.Errorf("canonicalizing SSH public key: %w", err)
		}

		canonicalKeys = append(canonicalKeys, canonicalKey)
	}

	slices.Sort(canonicalKeys)

	return canonicalKeys, nil
}
