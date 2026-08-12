// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filetype

import (
	"context"

	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps a FileType CR to a FileTypeData DTO and derives identity keys.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*filev1.FileType, *FileTypeData, FileTypeKey] = (*Translator)(nil)

// ShouldSkip returns false — FileType CRs are always syncable.
func (t *Translator) ShouldSkip(_ *filev1.FileType) (bool, string) {
	return false, ""
}

// Translate converts a FileType CR into a FileTypeData DTO.
func (t *Translator) Translate(_ context.Context, obj *filev1.FileType) (*FileTypeData, error) {
	phase, message := shared.StatusFromConditions(obj.Status.Conditions)

	active := obj.Status.FileExposureRef != nil

	var sftpInstanceName *string
	var sftpInstanceNamespace *string
	if obj.Status.SFTPInstance != nil {
		sftpInstanceName = &obj.Status.SFTPInstance.Name
		sftpInstanceNamespace = &obj.Status.SFTPInstance.Namespace
	}

	return &FileTypeData{
		Meta:                  shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		StatusPhase:           phase,
		StatusMessage:         message,
		FileType:              obj.Name,
		Description:           obj.Spec.Description,
		Variant:               nil,
		Active:                active,
		SFTPInstanceName:      sftpInstanceName,
		SFTPInstanceNamespace: sftpInstanceNamespace,
	}, nil
}

// KeyFromObject derives the composite identity key from a live FileType CR.
func (t *Translator) KeyFromObject(obj *filev1.FileType) FileTypeKey {
	return FileTypeKey{
		FileType: obj.Name,
	}
}

// KeyFromDelete derives the identity key for a delete operation.
// If lastKnown is available, it uses metadata-derived values.
// Otherwise, it falls back to req.Name.
func (t *Translator) KeyFromDelete(req types.NamespacedName, lastKnown *filev1.FileType) (FileTypeKey, error) {
	if lastKnown != nil {
		return FileTypeKey{
			FileType: lastKnown.Name,
		}, nil
	}
	return FileTypeKey{
		FileType: req.Name,
	}, nil
}
