// Copyright 2025 Deutsche Telekom IT GmbH
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

func CanonicalSSHPublicKeys(publicKeys []filev1.SSHPublicKeySpec) ([]string, error) {
	byFingerprint := make(map[string]string, len(publicKeys))
	fingerprints := make([]string, 0, len(publicKeys))

	for i := range publicKeys {
		canonicalKey, err := sftpv1.CanonicalPublicKey(publicKeys[i].Key)
		if err != nil {
			return nil, fmt.Errorf("canonicalizing SSH public key: %w", err)
		}

		fingerprint, err := sftpv1.FingerprintForKey(canonicalKey)
		if err != nil {
			return nil, fmt.Errorf("fingerprinting SSH public key: %w", err)
		}

		existingKey, exists := byFingerprint[fingerprint]
		if exists && existingKey != canonicalKey {
			return nil, fmt.Errorf("fingerprint %q is already assigned to another SSH public key", fingerprint)
		}
		if exists {
			continue
		}

		byFingerprint[fingerprint] = canonicalKey
		fingerprints = append(fingerprints, fingerprint)
	}

	slices.Sort(fingerprints)
	keys := make([]string, 0, len(fingerprints))
	for i := range fingerprints {
		keys = append(keys, byFingerprint[fingerprints[i]])
	}
	return keys, nil
}
