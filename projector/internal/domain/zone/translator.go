// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"strings"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Translator maps a Zone CR to a ZoneData DTO and derives identity keys.
// Zone uses the Strong delete strategy — KeyFromDelete always succeeds.
type Translator struct{}

// compile-time interface check.
var _ runtime.Translator[*adminv1.Zone, *ZoneData, ZoneKey] = (*Translator)(nil)

// ShouldSkip returns false — Zone CRs are always syncable.
func (t *Translator) ShouldSkip(_ *adminv1.Zone) (bool, string) {
	return false, ""
}

// Translate converts a Zone CR into a ZoneData DTO.
// Visibility is converted from title case ("World"/"Enterprise") to
// upper case ("WORLD"/"ENTERPRISE") to match the ent enum.
// GatewayURL is taken from Spec.Gateway.Url; an empty string is mapped to nil.
func (t *Translator) Translate(_ context.Context, obj *adminv1.Zone) (*ZoneData, error) {
	var gatewayURL *string
	if obj.Spec.Gateway.Url != "" {
		url := obj.Spec.Gateway.Url
		gatewayURL = &url
	}

	return &ZoneData{
		Meta:       shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		Name:       obj.Name,
		GatewayURL: gatewayURL,
		Visibility: strings.ToUpper(string(obj.Spec.Visibility)),
	}, nil
}

// KeyFromObject derives the identity key from a live Zone object.
func (t *Translator) KeyFromObject(obj *adminv1.Zone) ZoneKey {
	return ZoneKey(obj.Name)
}

// KeyFromDelete derives the identity key for a delete operation.
// Zone uses the Strong strategy — the key is always derivable from req.Name.
func (t *Translator) KeyFromDelete(req types.NamespacedName, _ *adminv1.Zone) (ZoneKey, error) {
	return ZoneKey(req.Name), nil
}
