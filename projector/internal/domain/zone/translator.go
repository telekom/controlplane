// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
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
// The URLs come from the zone's representative (API-typed default) profile — ZoneData
// carries no traffic kind. They stay nil when the zone has no such profile, whether
// because no preset is configured, none is API-typed, or none of those is the default.
// This is a read model, so a bad zone degrades to a partial row rather than blocking the
// projection; the miss is logged because ambiguous or missing defaults mean misconfiguration.
func (t *Translator) Translate(ctx context.Context, obj *adminv1.Zone) (*ZoneData, error) {
	var gatewayURL, issuerURL, permissionsURL *string

	preset, err := obj.Spec.GetDefaultPreset()
	if err != nil {
		logr.FromContextOrDiscard(ctx).V(1).Info("Zone has no representative preset, projecting URLs as null",
			"zone", obj.Name, "reason", err.Error())
	} else {
		url := preset.GetDefaultURL()
		gatewayURL = &url

		status, statusErr := obj.Status.GetPreset(preset.Name)
		if statusErr != nil {
			logr.FromContextOrDiscard(ctx).V(1).Info("Zone has no status for its representative preset, projecting issuer URLs as null",
				"zone", obj.Name, "preset", preset.Name, "reason", statusErr.Error())
		} else {
			if status.Links.Issuer != "" {
				u := status.Links.Issuer
				issuerURL = &u
			}
			if status.Links.PermissionsUrl != "" {
				u := status.Links.PermissionsUrl
				permissionsURL = &u
			}
		}
	}

	return &ZoneData{
		Meta:           shared.NewMetadata(obj.Namespace, obj.Name, obj.Labels),
		Name:           obj.Name,
		GatewayURL:     gatewayURL,
		IssuerURL:      issuerURL,
		PermissionsURL: permissionsURL,
		Visibility:     strings.ToUpper(string(obj.Spec.Visibility)),
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
