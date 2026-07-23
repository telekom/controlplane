// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package kgateway contains the Gateway-API-based counterpart to the Kong
// FeaturesBuilder. Instead of pushing raw Envoy xDS (see the envoy package) or
// calling the Kong admin API, it renders Gateway-API resources (HTTPRoute) plus
// kgateway CRs (Backend) and applies them as Kubernetes objects to a target
// cluster. kgateway's own controller then translates those CRs into Envoy
// config.
//
// The target cluster may be remote (not the cluster the operator runs on), so
// the write seam takes an arbitrary controller-runtime client rather than the
// operator's own client.
package kgateway

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kgwv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// AddToScheme registers the Gateway-API and kgateway types a target-cluster
// client must understand to apply the resources this package emits.
func AddToScheme(s *runtime.Scheme) error {
	if err := gwapiv1.Install(s); err != nil {
		return fmt.Errorf("installing gateway-api v1 scheme: %w", err)
	}
	if err := kgwv1alpha1.AddToScheme(s); err != nil {
		return fmt.Errorf("installing kgateway v1alpha1 scheme: %w", err)
	}
	return nil
}

// ResourceBundle is the set of resources emitted for a single Build. It is the
// kgateway analog of the envoy package's ResourceBundle.
type ResourceBundle struct {
	// Objects are the rendered CRs to apply, in apply order.
	Objects []client.Object
}

// Client is the write seam, analog of envoy.XdsClient and client.KongClient. It
// applies a bundle of resources to the target cluster.
type Client interface {
	Apply(ctx context.Context, bundle ResourceBundle) error
}

var _ Client = &crClient{}

type crClient struct {
	// c targets the cluster kgateway resources are provisioned on. This may be
	// a client for a remote cluster.
	c client.Client
	// fieldOwner identifies this operator as the server-side-apply field owner.
	fieldOwner client.FieldOwner
}

// NewClient wraps a controller-runtime client as a kgateway Client. The client
// may point at a remote cluster and its scheme must have the types registered
// via AddToScheme.
func NewClient(c client.Client) Client {
	return &crClient{c: c, fieldOwner: "gateway-operator"}
}

// Apply upserts every object in the bundle via server-side apply, which is
// idempotent and needs no read-modify-write.
func (c *crClient) Apply(ctx context.Context, bundle ResourceBundle) error {
	log := logr.FromContextOrDiscard(ctx).WithName("kgateway.client")

	for _, obj := range bundle.Objects {
		if err := c.c.Patch(ctx, obj, client.Apply, c.fieldOwner, client.ForceOwnership); err != nil {
			return fmt.Errorf("applying %T %s/%s: %w",
				obj, obj.GetNamespace(), obj.GetName(), err)
		}
		log.V(0).Info("Applied resource",
			"kind", fmt.Sprintf("%T", obj), "namespace", obj.GetNamespace(), "name", obj.GetName())
	}
	return nil
}
