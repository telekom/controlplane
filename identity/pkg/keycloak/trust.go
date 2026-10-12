// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/providerca"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

// CAResolver resolves provider CA references; it is the manager's shared CA cache.
type CAResolver = providerca.CAResolver[*commonclient.ProviderCABundle]

// ClientCache is the shared cache of Keycloak services.
type ClientCache = providerca.ClientCache[KeycloakService, *commonclient.ProviderCABundle]

// NewClientCache returns the Keycloak service cache resolving trust through ca.
func NewClientCache(ca CAResolver) *ClientCache {
	return providerca.NewClientCache[KeycloakService, *commonclient.ProviderCABundle](ca)
}

// ValidateProviderCA resolves ref through ca and maps failures to Identity controller errors.
// A nil ref selects system trust and needs no resolution.
func ValidateProviderCA(ctx context.Context, ca CAResolver, namespace string, ref *ctypes.CABundleConfigMapRef) error {
	if ref == nil {
		return nil
	}
	if _, err := ca.Resolve(ctx, namespace, ref); err != nil {
		return mapCAError(namespace, ref, err)
	}
	return nil
}

// mapCAError maps a shared CA resolution failure to an Identity controller error.
func mapCAError(namespace string, ref *ctypes.CABundleConfigMapRef, err error) error {
	re := providerca.AsResolveError(err)
	if re == nil {
		return ctrlerrors.RetryableErrorf("resolving CA bundle ConfigMap %s/%s: %s", namespace, ref.Name, err)
	}
	switch re.Kind {
	case providerca.FailureInvalidReference:
		return ctrlerrors.BlockedErrorf("%s", re.Err)
	case providerca.FailureNotFound:
		return ctrlerrors.BlockedErrorf("CA bundle ConfigMap %s/%s not found or lacks label %s=%s",
			namespace, ref.Name, ctypes.ProviderCALabelKey, ctypes.ProviderCALabelValue)
	case providerca.FailureForbidden:
		return ctrlerrors.BlockedErrorf("CA bundle ConfigMap %s/%s is not available: %s", namespace, ref.Name, apierrors.ReasonForError(re.Err))
	case providerca.FailureReadFailed:
		return ctrlerrors.RetryableErrorf("reading CA bundle ConfigMap %s/%s: %s", namespace, ref.Name, re.Err)
	case providerca.FailureInvalidBundle:
		return ctrlerrors.BlockedErrorf("CA bundle ConfigMap %s/%s key %q is invalid: %s", namespace, ref.Name, ref.Key, re.Err)
	default:
		return ctrlerrors.BlockedErrorf("CA bundle ConfigMap %s/%s %s", namespace, ref.Name, re.Err)
	}
}
