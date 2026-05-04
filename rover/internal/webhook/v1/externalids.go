// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"fmt"
	"regexp"
	"sync"

	"k8s.io/apimachinery/pkg/util/validation/field"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
)

// patternCache stores compiled regexes keyed by pattern string. The universe of
// patterns is bounded by the Zone CRs in the cluster (each with MaxItems=16
// policies), so this cache is effectively static after warm-up.
var patternCache sync.Map

func compilePattern(pattern string) (*regexp.Regexp, error) {
	if re, ok := patternCache.Load(pattern); ok {
		return re.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	patternCache.Store(pattern, re)
	return re, nil
}

// externalIdEntry decouples the check from the rover/application typed structs
// so the same helper serves both webhooks.
type externalIdEntry struct {
	Scheme string
	Id     string
}

// validateExternalIds enforces the zone's ExternalIdPolicies against the
// supplied externalIds:
//   - Required=true + scheme missing → required error on spec.externalIds.
//   - Scheme supplied + pattern mismatch → invalid error on
//     spec.externalIds[i].id.
//
// Entries whose scheme has no matching zone policy pass through unvalidated.
func validateExternalIds(valErr *cerrors.ValidationError, entries []externalIdEntry, zone *adminv1.Zone, base *field.Path) {
	if zone == nil || len(zone.Spec.ExternalIdPolicies) == 0 {
		return
	}

	index := make(map[string]int, len(entries))
	for i, e := range entries {
		index[e.Scheme] = i
	}

	for _, policy := range zone.Spec.ExternalIdPolicies {
		idx, present := index[policy.Scheme]
		if !present {
			if policy.Required {
				valErr.AddRequiredError(
					base,
					fmt.Sprintf("externalIds entry with scheme %q is required in zone %q", policy.Scheme, zone.Name),
				)
			}
			continue
		}

		re, err := compilePattern(policy.Pattern)
		if err != nil {
			valErr.AddInvalidError(
				base.Index(idx).Child("id"),
				entries[idx].Id,
				fmt.Sprintf("zone %q has an invalid pattern for scheme %q: %v", zone.Name, policy.Scheme, err),
			)
			continue
		}

		if !re.MatchString(entries[idx].Id) {
			valErr.AddInvalidError(
				base.Index(idx).Child("id"),
				entries[idx].Id,
				fmt.Sprintf("must match pattern %q for scheme %q in zone %q", policy.Pattern, policy.Scheme, zone.Name),
			)
		}
	}
}
