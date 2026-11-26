// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiroadmap

import (
	"context"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps an ApiRoadmap CR to Data and derives identity keys.
type Translator struct{}

var _ runtime.Translator[*apiv1.ApiRoadmap, *Data, Key] = (*Translator)(nil)

// ShouldSkip skips roadmaps whose file has not been uploaded yet.
func (t *Translator) ShouldSkip(obj *apiv1.ApiRoadmap) (bool, string) {
	if obj.Spec.Contents == "" {
		return true, "roadmap has no file ID"
	}
	return false, ""
}

// Translate converts an ApiRoadmap CR into Data.
func (t *Translator) Translate(_ context.Context, obj *apiv1.ApiRoadmap) (*Data, error) {
	return &Data{
		Meta:      shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		BasePath:  obj.Spec.BasePath,
		Namespace: obj.Namespace,
		TeamName:  shared.TeamNameFromNamespace(obj.Namespace),
		FileID:    obj.Spec.Contents,
	}, nil
}

// KeyFromObject derives the identity key from a live ApiRoadmap CR.
func (t *Translator) KeyFromObject(obj *apiv1.ApiRoadmap) Key {
	return Key{
		BasePath:  obj.Spec.BasePath,
		Namespace: obj.Namespace,
		TeamName:  shared.TeamNameFromNamespace(obj.Namespace),
	}
}

// KeyFromDelete needs the last known object because the CR name is not the base path.
func (t *Translator) KeyFromDelete(_ types.NamespacedName, lastKnown *apiv1.ApiRoadmap) (Key, error) {
	if lastKnown == nil {
		return Key{}, runtime.ErrDeleteKeyLost
	}
	return t.KeyFromObject(lastKnown), nil
}
