// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func ChildLabels(fileTypeRef types.ObjectRef) map[string]string {
	return map[string]string{
		config.DomainLabelKey:              "file",
		filev1.FileTypeNameLabelKey:        labelutil.NormalizeLabelValue(fileTypeRef.Name),
		filev1.FileTypeNamespaceLabelKey:   labelutil.NormalizeLabelValue(fileTypeRef.Namespace),
		config.BuildLabelKey("file.type"):  labelutil.NormalizeLabelValue(fileTypeRef.Name),
		config.BuildLabelKey("managed.by"): "file-operator",
	}
}

// FileTypeLabelSelector returns a selector matching resources labeled for the given FileType name.
func FileTypeLabelSelector(fileTypeName string) labels.Selector {
	return labels.SelectorFromSet(labels.Set{
		filev1.FileTypeNameLabelKey: labelutil.NormalizeLabelValue(fileTypeName),
	})
}
