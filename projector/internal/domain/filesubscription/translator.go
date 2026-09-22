// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filesubscription

import (
	"context"

	"k8s.io/apimachinery/pkg/types"

	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
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

	return &FileSubscriptionData{
		Meta:               shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:        phase,
		StatusMessage:      message,
		Zone:               obj.Spec.Zone.Name,
		FileSFTP:           toFileSFTP(obj.Spec.SFTP),
		ServiceURL:         obj.Status.ServiceURL,
		ServiceExternalURL: obj.Status.ServiceExternalURL,
		OwnerAppName:       obj.Spec.Requestor.Name,
		OwnerTeamName:      shared.TeamNameFromNamespace(obj.Namespace),
		TargetFileType:     obj.Spec.FileType,
	}, nil
}

func toFileSFTP(obj *filev1.FileSFTP) *model.FileSFTP {
	if obj == nil {
		return nil
	}

	if len(obj.PublicKeys) == 0 {
		return nil
	}

	sftp := model.FileSFTP{PublicKeys: make([]model.SSHPublicKeySpec, 0)}
	for i := range obj.PublicKeys {
		sftp.PublicKeys = append(sftp.PublicKeys, model.SSHPublicKeySpec{
			Key: obj.PublicKeys[i].Key,
		})
	}
	return &sftp
}

// KeyFromObject derives the composite identity key from a live FileSubscription.
func (t *Translator) KeyFromObject(obj *filev1.FileSubscription) FileSubscriptionKey {
	return FileSubscriptionKey{
		FileType:      obj.Spec.FileType,
		OwnerAppName:  obj.Spec.Requestor.AppName,
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
			OwnerAppName:  lastKnown.Spec.Requestor.Name,
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
