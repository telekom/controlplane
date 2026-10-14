// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cabundle

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/types"
)

// SourceError wraps an error related to a referenced ConfigMap key.
type SourceError struct {
	Namespace string
	Name      string
	Key       string
	Err       error
}

func (e *SourceError) Error() string {
	return fmt.Sprintf("CA bundle source configmap %s/%s key %q: %v", e.Namespace, e.Name, e.Key, e.Err)
}

func (e *SourceError) Unwrap() error {
	return e.Err
}

// ReadSource reads and validates the PEM content of a referenced ConfigMap key.
// Only data is considered, binaryData is ignored.
//
// Errors match ErrSourceNotFound, ErrSourceForbidden or ErrInvalidBundle when
// caused by the source; any other error is a transient Kubernetes error.
func ReadSource(ctx context.Context, r client.Reader, namespace string, ref types.ConfigMapKeyRef) ([]byte, error) {
	srcErr := func(err error) error {
		return &SourceError{Namespace: namespace, Name: ref.Name, Key: ref.Key, Err: err}
	}
	if ref.Name == "" || ref.Key == "" {
		return nil, srcErr(invalidf("configmap name and key are required"))
	}

	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, cm); err != nil {
		switch {
		case apierrors.IsNotFound(err):
			return nil, srcErr(fmt.Errorf("%w: %w", ErrSourceNotFound, err))
		case apierrors.IsForbidden(err):
			return nil, srcErr(fmt.Errorf("%w: %w", ErrSourceForbidden, err))
		default:
			return nil, srcErr(fmt.Errorf("reading configmap: %w", err))
		}
	}

	value, ok := cm.Data[ref.Key]
	if !ok {
		return nil, srcErr(fmt.Errorf("%w: key not present in data", ErrSourceNotFound))
	}
	if _, err := parseSet([]byte(value)); err != nil {
		return nil, srcErr(err)
	}
	return []byte(value), nil
}

// ResolveRefs reads all referenced keys from namespace and returns their
// canonical union (see Canonicalize). Without refs, nil is returned.
func ResolveRefs(ctx context.Context, r client.Reader, namespace string, refs ...types.ConfigMapKeyRef) ([]byte, error) {
	sets := make([][]byte, 0, len(refs))
	for _, ref := range refs {
		data, err := ReadSource(ctx, r, namespace, ref)
		if err != nil {
			return nil, err
		}
		sets = append(sets, data)
	}
	return Canonicalize(sets...)
}

// Resolver resolves the canonical bundle of the given refs on every call.
// It structurally implements the common-server tlsclient.CAResolver interface.
type Resolver struct {
	Reader    client.Reader
	Namespace string
	Refs      []types.ConfigMapKeyRef
}

// NewResolver returns a Resolver for a single reference.
func NewResolver(r client.Reader, namespace string, ref types.ConfigMapKeyRef) *Resolver {
	return &Resolver{Reader: r, Namespace: namespace, Refs: []types.ConfigMapKeyRef{ref}}
}

// Resolve returns the validated canonical PEM bundle.
func (r *Resolver) Resolve(ctx context.Context) ([]byte, error) {
	if r == nil || r.Reader == nil {
		return nil, fmt.Errorf("CA bundle resolver is not configured")
	}
	return ResolveRefs(ctx, r.Reader, r.Namespace, r.Refs...)
}
