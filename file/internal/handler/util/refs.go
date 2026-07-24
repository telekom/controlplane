// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	filev1 "github.com/telekom/controlplane/file/api/v1"
)

func SFTPUserRefForFileType(fileType *filev1.FileType) types.ObjectRef {
	return types.ObjectRef{
		Name:      fileType.Name,
		Namespace: fileType.Namespace,
	}
}

func SFTPUserRefForFileSubscription(subscription *filev1.FileSubscription) types.ObjectRef {
	return types.ObjectRef{
		Name:      labelutil.NormalizeNameValue("filesubscription-" + subscription.Name),
		Namespace: subscription.Namespace,
	}
}

func SFTPUserRefForFileExposure(exposure *filev1.FileExposure) types.ObjectRef {
	return types.ObjectRef{
		Name:      labelutil.NormalizeNameValue("fileexposure-" + exposure.Name),
		Namespace: exposure.Namespace,
	}
}

func SFTPInstanceRefForFileExposure(exposure *filev1.FileExposure) types.ObjectRef {
	return types.ObjectRef{
		Name:      exposure.Spec.FileType,
		Namespace: exposure.Namespace,
	}
}

func SFTPServiceConfigRefForZoneServiceConfig(zoneServiceConfig *filev1.ZoneServiceConfig) types.ObjectRef {
	if zoneServiceConfig.Status.SFTPServiceConfigRef != nil && !zoneServiceConfig.Status.SFTPServiceConfigRef.IsEmpty() {
		return *zoneServiceConfig.Status.SFTPServiceConfigRef
	}
	return *types.ObjectRefFromObject(zoneServiceConfig)
}

func FileExposureSourceRef(exposure *filev1.FileExposure) types.TypedObjectRef {
	return types.TypedObjectRef{
		TypeMeta: metav1.TypeMeta{
			APIVersion: filev1.GroupVersion.String(),
			Kind:       "FileExposure",
		},
		ObjectRef: types.ObjectRef{
			Name:      exposure.Name,
			Namespace: exposure.Namespace,
		},
	}
}

func FileSubscriptionSourceRef(subscription *filev1.FileSubscription) types.TypedObjectRef {
	return types.TypedObjectRef{
		TypeMeta: metav1.TypeMeta{
			APIVersion: filev1.GroupVersion.String(),
			Kind:       "FileSubscription",
		},
		ObjectRef: types.ObjectRef{
			Name:      subscription.Name,
			Namespace: subscription.Namespace,
		},
	}
}
