// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filespecification

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ handler.Handler[*roverv1.FileSpecification] = (*FileSpecificationHandler)(nil)

// FileSpecificationHandler reconciles a rover-domain FileSpecification into a
// file-domain FileType (mirrors FileSpecification -> FileType).
type FileSpecificationHandler struct{}

func (h *FileSpecificationHandler) CreateOrUpdate(ctx context.Context, obj *roverv1.FileSpecification) error {
	c := cclient.ClientFromContextOrDie(ctx)

	fileType := &filev1.FileType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roverv1.MakeFileSpecificationName(obj),
			Namespace: obj.Namespace,
		},
	}

	obj.Status.FileType = *types.ObjectRefFromObject(fileType)

	mutator := func() error {
		if err := controllerutil.SetControllerReference(obj, fileType, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		fileType.Spec = filev1.FileTypeSpec{
			Description: obj.Spec.Description,
		}
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, fileType, mutator); err != nil {
		return errors.Wrap(err, "failed to create or update FileType")
	}

	if c.AnyChanged() {
		obj.SetCondition(condition.NewProcessingCondition(condition.ReasonProcessing, "FileType updated"))
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonProvisioning, "FileType is not ready"))
	} else {
		obj.SetCondition(condition.NewDoneProcessingCondition("FileType created"))
		obj.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "FileType is ready"))
	}

	return nil
}

func (h *FileSpecificationHandler) Delete(ctx context.Context, obj *roverv1.FileSpecification) error {
	fileType := &filev1.FileType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roverv1.MakeFileSpecificationName(obj),
			Namespace: obj.Namespace,
		},
	}

	cc := cclient.ClientFromContextOrDie(ctx)
	err := cc.Get(ctx, client.ObjectKeyFromObject(fileType), fileType)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get FileType before deletion: %w", err)
	}

	return ctrlerrors.BlockedErrorf("fileType still exists")
}
