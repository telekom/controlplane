// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
)

const changelogDateLayout = "2006-01-02"

// semverRe is the official regular expression from https://semver.org
var semverRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// validateChangelogItems checks the formats required by the API changelog documentation:
// date as yyyy-MM-dd, version as MAJOR.MINOR.PATCH and a non-empty description.
func validateChangelogItems(items []api.ApiChangelogItem) error {
	var violations []string
	for i, item := range items {
		if _, err := time.Parse(changelogDateLayout, item.Date); err != nil {
			violations = append(violations, fmt.Sprintf("items[%d].date %q must have the format yyyy-MM-dd", i, item.Date))
		}
		if !semverRe.MatchString(item.Version) {
			violations = append(violations, fmt.Sprintf("items[%d].version %q must have the format MAJOR.MINOR.PATCH", i, item.Version))
		}
		if strings.TrimSpace(item.Description) == "" {
			violations = append(violations, fmt.Sprintf("items[%d].description must not be empty", i))
		}
	}
	if len(violations) > 0 {
		return problems.BadRequest(strings.Join(violations, "; "))
	}
	return nil
}

// requireFileManager fails early when the file manager is disabled, before any store is consulted.
func requireFileManager(resource string) error {
	if !cconfig.FeatureFileManager.IsEnabled() {
		return problems.BadRequest(resource + " requires the file manager, which is disabled in this environment")
	}
	return nil
}

// ensureApiSpecificationExists rejects changelogs and roadmaps for an API that was not provided via Rover yet.
func ensureApiSpecificationExists(ctx context.Context, stores *s.Stores, id mapper.ResourceIdInfo, basePath string) error {
	// id comes from mapper.ParseResourceId, which sets Environment from the authenticated business context.
	ns := id.Environment + "--" + id.Namespace
	apiSpec, err := stores.APISpecificationStore.Get(ctx, ns, labelutil.NormalizeNameValue(basePath))
	if err == nil {
		// The name is normalized, so different paths (e.g. differing only in case) can map to the same spec.
		if apiSpec.Spec.BasePath != basePath {
			return problems.BadRequest("ApiSpecification " + apiSpec.Name + " has basePath " + apiSpec.Spec.BasePath + ", not " + basePath)
		}
		return nil
	}
	if problems.IsNotFound(err) {
		return problems.BadRequest("no ApiSpecification exists for basePath " + basePath + ", apply it first")
	}
	return err
}
