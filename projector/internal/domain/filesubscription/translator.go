// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filesubscription

import (
	"context"

	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	applicationLabelKey = "cp.ei.telekom.de/application"
)

// Translator maps a FileSubscription CR to a FileSubscriptionData DTO.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*filev1.FileSubscription, *FileSubscriptionData, FileSubscriptionKey] = (*Translator)(nil)

// ShouldSkip returns false — FileSubscription CRs are always syncable.
func (t *Translator) ShouldSkip(obj *filev1.FileSubscription) (bool, string) {
	return false, ""
}

// Translate converts a FileSubscription CR into a FileSubscriptionData DTO.
func (t *Translator) Translate(_ context.Context, obj *filev1.FileSubscription) (*FileSubscriptionData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	publicKeys := []string{}
	if obj.Spec.SFTP != nil {
		for i := range obj.Spec.SFTP.PublicKeys {
			publicKeys = append(publicKeys, obj.Spec.SFTP.PublicKeys[i].Key)
		}
	}

	return &FileSubscriptionData{
		Meta:           shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:    phase,
		StatusMessage:  message,
		Zone:           obj.Spec.Zone.Name,
		SFTPPublicKeys: publicKeys,
		OwnerAppName:   obj.Labels[applicationLabelKey],
		OwnerTeamName:  shared.TeamNameFromNamespace(obj.Namespace),
		TargetFileType: obj.Spec.FileType,
	}, nil
}

// KeyFromObject derives the composite identity key from a live FileSubscription.
func (t *Translator) KeyFromObject(obj *filev1.FileSubscription) FileSubscriptionKey {
	return FileSubscriptionKey{
		FileType:      obj.Spec.FileType,
		OwnerAppName:  obj.Labels[applicationLabelKey],
		OwnerTeamName: shared.TeamNameFromNamespace(obj.Namespace),
		Namespace:     obj.Namespace,
		Name:          obj.Name,
	}
}

// KeyFromDelete derives the identity key for a delete operation.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *filev1.FileSubscription) (FileSubscriptionKey, error) {
	if lastKnown != nil {
		return FileSubscriptionKey{
			FileType:      lastKnown.Spec.FileType,
			OwnerAppName:  lastKnown.Labels[applicationLabelKey],
			OwnerTeamName: shared.TeamNameFromNamespace(lastKnown.Namespace),
			Namespace:     lastKnown.Namespace,
			Name:          lastKnown.Name,
		}, nil
	}
	return FileSubscriptionKey{
		FileType:      req.Name,
		OwnerAppName:  req.Name,
		OwnerTeamName: shared.TeamNameFromNamespace(req.Namespace),
		Namespace:     req.Namespace,
		Name:          req.Name,
	}, nil
}
