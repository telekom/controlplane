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
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cerrors "github.com/telekom/controlplane/common/pkg/errors"
)

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

// validateExternalIds enforces the zone's ExternalIdPolicies against the
// Application's externalIds. Parallels the Rover webhook's implementation.
func validateExternalIds(valErr *cerrors.ValidationError, entries []applicationv1.ExternalId, zone *adminv1.Zone, base *field.Path) {
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
