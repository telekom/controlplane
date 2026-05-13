// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"strings"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
)

// isBasepathWhitelisted checks if the given basepath is whitelisted in the category-level linting config.
func isBasepathWhitelisted(lintCfg *apiv1.LintingConfig, basepath string) bool {
	for _, wl := range lintCfg.WhitelistedBasepaths {
		if strings.EqualFold(wl, basepath) {
			return true
		}
	}
	return false
}

// lintingConfigFromList finds the linting configuration from a pre-fetched ApiCategoryList.
// Returns nil if no linting is configured for this category.
func lintingConfigFromList(categoryList *apiv1.ApiCategoryList, category string) *apiv1.LintingConfig {
	if categoryList == nil {
		return nil
	}

	found, ok := categoryList.FindByLabelValue(category)
	if !ok {
		return nil
	}

	return found.Spec.Linting
}
