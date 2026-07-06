// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filespecification

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ handler.Handler[*roverv1.FileSpecification] = (*FileSpecificationHandler)(nil)

// FileSpecificationHandler reconciles a rover-domain FileSpecification into a
// file-domain FileType (mirrors EventSpecificationHandler -> EventType).
type FileSpecificationHandler struct{}

func (h *FileSpecificationHandler) CreateOrUpdate(ctx context.Context, fileSpec *roverv1.FileSpecification) error {
	c := client.ClientFromContextOrDie(ctx)

	// The FileType name is derived from the FileSpecification name (the file type
	// identifier); they live in the same namespace.
	name := roverv1.MakeFileSpecificationName(fileSpec)

	fileType := &filev1.FileType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(name),
			Namespace: fileSpec.Namespace,
		},
	}

	fileSpec.Status.FileType = *types.ObjectRefFromObject(fileType)

	mutator := func() error {
		if err := controllerutil.SetControllerReference(fileSpec, fileType, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		fileType.Labels = map[string]string{
			filev1.FileTypeLabelKey: labelutil.NormalizeLabelValue(fileSpec.Name),
		}

		fileType.Spec = filev1.FileTypeSpec{
			Type:          fileSpec.Name,
			Description:   fileSpec.Spec.Description,
			Specification: fileSpec.Spec.Specification,
		}
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, fileType, mutator); err != nil {
		return errors.Wrap(err, "failed to create or update FileType")
	}

	if c.AnyChanged() {
		fileSpec.SetCondition(condition.NewProcessingCondition("Provisioning", "FileType updated"))
		fileSpec.SetCondition(condition.NewNotReadyCondition("Provisioning", "FileType is not ready"))
	} else {
		fileSpec.SetCondition(condition.NewDoneProcessingCondition("FileType created"))
		fileSpec.SetCondition(condition.NewReadyCondition("Provisioned", "FileType is ready"))
	}

	return nil
}

func (h *FileSpecificationHandler) Delete(ctx context.Context, obj *roverv1.FileSpecification) error {
	return nil
}
