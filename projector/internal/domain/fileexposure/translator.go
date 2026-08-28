// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fileexposure

import (
	"context"
	"strings"

	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
)

const (
	applicationLabelKey = "cp.ei.telekom.de/application"
)

// Translator maps a FileExposure CR to a FileExposureData DTO and derives keys.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*filev1.FileExposure, *FileExposureData, FileExposureKey] = (*Translator)(nil)

// ShouldSkip returns false — FileExposure CRs are always syncable.
func (t *Translator) ShouldSkip(obj *filev1.FileExposure) (bool, string) {
	return false, ""
}

// Translate converts a FileExposure CR into a FileExposureData DTO.
func (t *Translator) Translate(_ context.Context, obj *filev1.FileExposure) (*FileExposureData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	ownerAppName := obj.Labels[applicationLabelKey]

	var provider *string
	if obj.Spec.Provider != "" {
		provider = &obj.Spec.Provider
	}

	publicKeys := []string{}
	if obj.Spec.SFTP != nil {
		for i := range obj.Spec.SFTP.PublicKeys {
			publicKeys = append(publicKeys, obj.Spec.SFTP.PublicKeys[i].Key)
		}
	}

	return &FileExposureData{
		Meta:           shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:    phase,
		StatusMessage:  message,
		Provider:       provider,
		Visibility:     strings.ToUpper(string(obj.Spec.Visibility)),
		Active:         isActiveExposure(obj),
		Zone:           obj.Spec.Zone.Name,
		SFTPPublicKeys: publicKeys,
		ApprovalConfig: model.ApprovalConfig{
			Strategy:     shared.MapApprovalStrategy(string(obj.Spec.Approval.Strategy)),
			TrustedTeams: obj.Spec.Approval.TrustedTeams,
		},
		AppName:        ownerAppName,
		TeamName:       shared.TeamNameFromNamespace(obj.Namespace),
		TargetFileType: obj.Spec.FileType,
	}, nil
}

func isActiveExposure(obj *filev1.FileExposure) bool {
	ready := meta.FindStatusCondition(obj.Status.Conditions, "Ready")
	if ready != nil && ready.Reason == "FileExposureAlreadyExists" {
		return false
	}
	return true
}

// KeyFromObject derives the composite identity key from a live FileExposure CR.
func (t *Translator) KeyFromObject(obj *filev1.FileExposure) FileExposureKey {
	return FileExposureKey{
		FileType: obj.Spec.FileType,
		AppName:  obj.Labels[applicationLabelKey],
		TeamName: shared.TeamNameFromNamespace(obj.Namespace),
	}
}

// KeyFromDelete derives the identity key for a delete operation.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *filev1.FileExposure) (FileExposureKey, error) {
	if lastKnown != nil {
		return FileExposureKey{
			FileType: lastKnown.Spec.FileType,
			AppName:  lastKnown.Labels[applicationLabelKey],
			TeamName: shared.TeamNameFromNamespace(lastKnown.Namespace),
		}, nil
	}
	return FileExposureKey{
		FileType: req.Name,
		AppName:  req.Name,
		TeamName: shared.TeamNameFromNamespace(req.Namespace),
	}, nil
}
