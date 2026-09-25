// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Translator struct {
	Reader client.Reader
	// OnMissingApplication removes an old projection when its SpectreApplication is absent.
	OnMissingApplication func(context.Context, Key) error
}

var _ runtime.Translator[*spectrev1.Listener, *Data, Key] = (*Translator)(nil)

func (t *Translator) ShouldSkip(obj *spectrev1.Listener) (bool, string) {
	if obj.Spec.EventListener != nil || obj.Spec.ApiListener == nil || obj.Spec.ApiListener.ApiBasePath == "" {
		return true, "apiListener is not configured"
	}
	if obj.Spec.Application.Name == "" || obj.Spec.Consumer.Name == "" || obj.Spec.Provider.Name == "" ||
		obj.Spec.Consumer.Namespace == "" || obj.Spec.Provider.Namespace == "" {
		return true, "application, consumer or provider reference is incomplete"
	}
	return false, ""
}

func (t *Translator) Translate(ctx context.Context, obj *spectrev1.Listener) (*Data, error) {
	ref := obj.Spec.Application
	if ref.Namespace == "" {
		ref.Namespace = obj.Namespace
	}
	app := &spectrev1.SpectreApplication{}
	if err := t.Reader.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, app); err != nil {
		if apierrors.IsNotFound(err) {
			if t.OnMissingApplication != nil {
				if deleteErr := t.OnMissingApplication(ctx, Key{Namespace: obj.Namespace, Name: obj.Name}); deleteErr != nil {
					return nil, fmt.Errorf("remove listener after missing spectre_application %s: %w", ref.String(), deleteErr)
				}
			}
			return nil, runtime.WrapDependencyMissing("spectre_application", ref.String())
		}
		return nil, fmt.Errorf("get spectre_application %s: %w", ref.String(), err)
	}
	observer := app.Spec.Application.ObjectRef
	if observer.Name == "" || observer.Namespace == "" {
		if t.OnMissingApplication != nil {
			if err := t.OnMissingApplication(ctx, Key{Namespace: obj.Namespace, Name: obj.Name}); err != nil {
				return nil, fmt.Errorf("remove listener without observer: %w", err)
			}
		}
		return nil, runtime.WrapDependencyMissing("application", ref.String())
	}
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)
	data := &Data{
		Meta:          shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:   phase,
		StatusMessage: message,
		BasePath:      obj.Spec.ApiListener.ApiBasePath,
		Observer:      Key{Namespace: observer.Namespace, Name: observer.Name},
		Consumer:      Key{Namespace: obj.Spec.Consumer.Namespace, Name: obj.Spec.Consumer.Name},
		Provider:      Key{Namespace: obj.Spec.Provider.Namespace, Name: obj.Spec.Provider.Name},
	}
	if f := obj.Spec.ApiListener.RequestFilter; f != nil {
		data.RequestFilter = &model.ListenerFilter{Trigger: f.Trigger, Payload: f.Payload}
	}
	if f := obj.Spec.ApiListener.ResponseFilter; f != nil {
		data.ResponseFilter = &model.ListenerFilter{Trigger: f.Trigger, Payload: f.Payload}
	}
	return data, nil
}

func (t *Translator) KeyFromObject(obj *spectrev1.Listener) Key {
	return Key{Namespace: obj.Namespace, Name: obj.Name}
}

func (t *Translator) KeyFromDelete(req types.NamespacedName, _ *spectrev1.Listener) (Key, error) {
	return Key{Namespace: req.Namespace, Name: req.Name}, nil
}
