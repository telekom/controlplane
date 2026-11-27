// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"context"
	"fmt"
	"strings"

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

func sameEnvironmentNamespace(env, objNamespace, targetNamespace string) bool {
	if targetNamespace == objNamespace {
		return true
	}
	return env != "" && strings.HasPrefix(targetNamespace, env+"--")
}

func (t *Translator) Translate(ctx context.Context, obj *spectrev1.Listener) (*Data, error) {
	ref := obj.Spec.Application
	if ref.Namespace == "" {
		ref.Namespace = obj.Namespace
	}
	env := shared.EnvironmentFromLabels(obj.Labels)
	if !sameEnvironmentNamespace(env, obj.Namespace, ref.Namespace) {
		return nil, runtime.WrapDependencyMissing("spectre_application", ref.String())
	}
	app := &spectrev1.SpectreApplication{}
	// ref.Namespace is constrained above to obj.Namespace or the "<env>--" namespace
	// prefix of the Listener's environment; the label check below also verifies the label.
	if err := t.Reader.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, app); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, runtime.WrapDependencyMissing("spectre_application", ref.String())
		}
		return nil, fmt.Errorf("get spectre_application %s: %w", ref.String(), err)
	}
	if shared.EnvironmentFromLabels(app.Labels) != env {
		return nil, runtime.WrapDependencyMissing("spectre_application", ref.String())
	}
	observer := app.Spec.Application.ObjectRef
	if observer.Name == "" || observer.Namespace == "" || !sameEnvironmentNamespace(env, obj.Namespace, observer.Namespace) {
		return nil, runtime.WrapDependencyMissing("application", ref.String())
	}
	if !sameEnvironmentNamespace(env, obj.Namespace, obj.Spec.Consumer.Namespace) {
		return nil, runtime.WrapDependencyMissing("application", obj.Spec.Consumer.String())
	}
	if !sameEnvironmentNamespace(env, obj.Namespace, obj.Spec.Provider.Namespace) {
		return nil, runtime.WrapDependencyMissing("application", obj.Spec.Provider.String())
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
		data.RequestFilter = mapListenerFilter(f)
	}
	if f := obj.Spec.ApiListener.ResponseFilter; f != nil {
		data.ResponseFilter = mapListenerFilter(f)
	}
	return data, nil
}

func mapListenerFilter(f *spectrev1.ListenerFilter) *model.ListenerFilter {
	var trigger map[string]any
	if f.Trigger != nil {
		trigger = make(map[string]any, len(f.Trigger))
		for k, v := range f.Trigger {
			trigger[k] = v
		}
	}
	return &model.ListenerFilter{Trigger: trigger, Payload: f.Payload}
}

func (t *Translator) KeyFromObject(obj *spectrev1.Listener) Key {
	return Key{Namespace: obj.Namespace, Name: obj.Name}
}

func (t *Translator) KeyFromDelete(req types.NamespacedName, _ *spectrev1.Listener) (Key, error) {
	return Key{Namespace: req.Namespace, Name: req.Name}, nil
}
