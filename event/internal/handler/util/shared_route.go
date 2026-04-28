// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DeleteRouteIfExists fetches a Route by ObjectRef and deletes it if found.
// Returns nil if the Route is already gone (NotFound).
func DeleteRouteIfExists(ctx context.Context, ref *ctypes.ObjectRef) error {
	if ref == nil {
		return nil
	}

	c := cclient.ClientFromContextOrDie(ctx)

	route := &gatewayapi.Route{}
	err := c.Get(ctx, ref.K8s(), route)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get Route %q", ref.String())
	}

	if err := c.Delete(ctx, route); err != nil {
		return errors.Wrapf(err, "failed to delete Route %q", ref.String())
	}

	return nil
}

type Options struct {
	Owner         metav1.Object
	IsProxyTarget bool
}

type Option func(*Options)

func WithOwner(owner metav1.Object) Option {
	return func(o *Options) {
		o.Owner = owner
	}
}

func WithProxyTarget(isProxyTarget bool) Option {
	return func(o *Options) {
		o.IsProxyTarget = isProxyTarget
	}
}

func (o *Options) apply(ctx context.Context, route *gatewayapi.Route) error {
	c := cclient.ClientFromContextOrDie(ctx)
	if o.Owner != nil {
		return controllerutil.SetControllerReference(o.Owner, route, c.Scheme())
	}
	return nil
}
