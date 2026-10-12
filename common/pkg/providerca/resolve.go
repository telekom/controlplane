// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package providerca

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/types"
)

// FailureKind classifies why a provider CA reference could not be resolved.
type FailureKind int

const (
	// FailureInvalidReference means the reference has invalid name or key syntax.
	FailureInvalidReference FailureKind = iota + 1
	// FailureNotFound means the ConfigMap does not exist. Through a label-filtered informer
	// cache this includes ConfigMaps lacking the provider-ca label.
	FailureNotFound
	// FailureForbidden means reading the ConfigMap is forbidden.
	FailureForbidden
	// FailureReadFailed is any other, transient read failure.
	FailureReadFailed
	// FailureLabelMissing means the ConfigMap lacks the exact provider-ca label.
	FailureLabelMissing
	// FailureInvalidMetadata means provider CA copy provenance metadata is invalid.
	FailureInvalidMetadata
	// FailureKeyMissing means the referenced data key is absent.
	FailureKeyMissing
	// FailureInvalidBundle means the referenced data is not a valid PEM bundle.
	FailureInvalidBundle
)

// ResolveError reports a failed resolution. Err is the underlying cause: the API error for
// read failures, otherwise a description without the ConfigMap identity.
type ResolveError struct {
	Kind FailureKind
	Err  error
}

func (e *ResolveError) Error() string { return e.Err.Error() }

func (e *ResolveError) Unwrap() error { return e.Err }

// AsResolveError returns the ResolveError of err, or nil.
func AsResolveError(err error) *ResolveError {
	var re *ResolveError
	if errors.As(err, &re) {
		return re
	}
	return nil
}

// Resolved is a validated provider CA bundle together with its source identity.
type Resolved[B any] struct {
	Ref          types.CABundleConfigMapRef
	ConfigMapUID k8stypes.UID
	Bundle       B
}

// ParseFunc strictly parses the PEM data of a referenced key.
type ParseFunc[B any] func(data []byte) (B, error)

// Resolve reads the referenced ConfigMap in namespace with reader, validates its label and provenance metadata, requires the
// referenced key and parses it with parse. A nil ref selects system trust: Resolve returns nil
// without reading. All failures are *ResolveError; callers map them to their own errors.
func Resolve[B any](ctx context.Context, reader client.Reader, namespace string,
	ref *types.CABundleConfigMapRef, parse ParseFunc[B],
) (*Resolved[B], error) {
	if ref == nil {
		return nil, nil
	}
	if err := ref.Validate(); err != nil {
		return nil, &ResolveError{Kind: FailureInvalidReference, Err: err}
	}
	cm := &corev1.ConfigMap{}
	if err := reader.Get(ctx, k8stypes.NamespacedName{Namespace: namespace, Name: ref.Name}, cm); err != nil {
		switch {
		case apierrors.IsNotFound(err):
			return nil, &ResolveError{Kind: FailureNotFound, Err: err}
		case apierrors.IsForbidden(err):
			return nil, &ResolveError{Kind: FailureForbidden, Err: err}
		default:
			return nil, &ResolveError{Kind: FailureReadFailed, Err: err}
		}
	}
	if !types.HasProviderCALabel(cm.Labels) {
		return nil, &ResolveError{
			Kind: FailureLabelMissing,
			Err:  fmt.Errorf("lacks label %s=%s", types.ProviderCALabelKey, types.ProviderCALabelValue),
		}
	}
	if err := ValidateProviderCAConfigMap(cm, ref); err != nil {
		return nil, &ResolveError{Kind: FailureInvalidMetadata, Err: err}
	}
	data, ok := cm.Data[ref.Key]
	if !ok {
		return nil, &ResolveError{Kind: FailureKeyMissing, Err: fmt.Errorf("has no key %q", ref.Key)}
	}
	bundle, err := parse([]byte(data))
	if err != nil {
		return nil, &ResolveError{Kind: FailureInvalidBundle, Err: err}
	}
	return &Resolved[B]{Ref: *ref, ConfigMapUID: cm.UID, Bundle: bundle}, nil
}
