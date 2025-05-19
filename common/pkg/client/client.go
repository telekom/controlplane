// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ScopedClient interface {
	Scheme() *runtime.Scheme
	CreateOrUpdate(ctx context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error)
	Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
	Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
	// AnyChanged returns true if any object has been created or updated
	AnyChanged() bool
	// Ready returns true if all objects are ready
	AllReady() bool
	// Reset resets the state of this client instance
	Reset()
}

var _ ScopedClient = &scopedClientImpl{}

type scopedClientImpl struct {
	environment string
	client.Client
	changed bool
	ready   bool
}

func NewScopedClient(c client.Client, environment string) ScopedClient {
	return &scopedClientImpl{
		Client:      c,
		environment: environment,
		changed:     false,
		ready:       true,
	}
}

func (e *scopedClientImpl) CreateOrUpdate(ctx context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
	wrapMutate := func() error {
		if err := mutate(); err != nil {
			return err
		}
		if obj.GetLabels() == nil {
			obj.SetLabels(map[string]string{})
		}
		obj.GetLabels()[config.EnvironmentLabelKey] = e.environment
		return nil
	}

	res, err := controllerutil.CreateOrUpdate(ctx, e.Client, obj, wrapMutate)
	if err != nil {
		return res, errors.Wrapf(err, "failed to create or update object %s", obj.GetName())
	}
	e.setChanged(res)
	e.setReady(obj)

	return res, nil
}

func (c *scopedClientImpl) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	if err != nil {
		return errors.Wrap(err, "failed to delete object")
	}

	labels := obj.GetLabels()
	if labels != nil && labels[config.EnvironmentLabelKey] != c.environment {
		return errors.New("object does not belong to the environment")
	}

	err = c.Client.Delete(ctx, obj, opts...)
	if err != nil {
		return errors.Wrap(err, "failed to delete object")
	}

	return nil
}

func (c *scopedClientImpl) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key.Namespace == "" {
		key.Namespace = c.environment
	}
	err := c.Client.Get(ctx, key, obj, opts...)
	if err != nil {
		return errors.Wrap(err, "failed to get object")
	}

	labels := obj.GetLabels()
	if labels == nil {
		obj = nil
		return errors.New("failed to get object: object does not have labels")
	}
	if labels[config.EnvironmentLabelKey] != c.environment {
		obj = nil
		return errors.New("failed to get object: object does not belong to the environment")
	}

	return nil
}

func (c *scopedClientImpl) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	opts = append(opts, client.MatchingLabels{config.EnvironmentLabelKey: c.environment})
	return c.Client.List(ctx, list, opts...)
}

// Reset resets the state of this client instance
func (c *scopedClientImpl) Reset() {
	c.changed = false
	c.ready = true
}

func (c *scopedClientImpl) AnyChanged() bool {
	return c.changed
}

func (c *scopedClientImpl) AllReady() bool {
	return c.ready
}

// setChanged will set the current client to changed if the operation result is not none
// If any object has been created or updated, the client will be marked as changed
func (c *scopedClientImpl) setChanged(res controllerutil.OperationResult) {
	if c.changed {
		return
	}
	if res != controllerutil.OperationResultNone {
		c.changed = true
	}
}

// setReady will set the current client to not ready if the object is not ready
// If any object is not ready, the client will be marked as not ready
func (c *scopedClientImpl) setReady(obj client.Object) {
	if !c.ready || obj == nil {
		return
	}

	if cobj, ok := obj.(types.Object); ok {
		if meta.IsStatusConditionFalse(cobj.GetConditions(), condition.ConditionTypeReady) {
			c.ready = false
		}
	}
}
