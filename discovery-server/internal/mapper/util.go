// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ApplicationLabelPath is the gjson path used in store.Filter to match the
	// application label. Dots in the label key must be escaped for gjson.
	ApplicationLabelPath = `metadata.labels.cp\.ei\.telekom\.de/application`

	// ApplicationLabelKey is the raw Kubernetes label key used at runtime to
	// read/verify the application label on an object.
	ApplicationLabelKey = "cp.ei.telekom.de/application"
)

// nsRE matches a namespace in the format <env>--<group>--<team>.
var nsRE = regexp.MustCompile(`^([a-z0-9-]+)--([a-z0-9-]+)--([a-z0-9-]+)$`)

// appIdRE matches an applicationId in the format <group>--<team>--<appName>.
var appIdRE = regexp.MustCompile(`^([a-z0-9-]+)--([a-z0-9-]+)--([a-z0-9-]+)$`)

// resourceIdRE matches a resourceId in the format <group>--<team>--<resourceName>.
var resourceIdRE = regexp.MustCompile(`^([a-z0-9-]+)--([a-z0-9-]+)--([a-z0-9-]+)$`)

// NamespaceInfo holds parsed components of a Kubernetes namespace.
type NamespaceInfo struct {
	Environment string
	Group       string
	Team        string
}

// ResourceIdInfo holds the parsed components of a resourceId path parameter.
type ResourceIdInfo struct {
	Namespace string
	Name      string
}

// ApplicationIdInfo holds the parsed components of an applicationId path parameter.
type ApplicationIdInfo struct {
	Environment string
	Group       string
	Team        string
	AppName     string
	Namespace   string // <env>--<group>--<team>
}

// ParseNamespace extracts environment, group, and team from a namespace
// in the format <env>--<group>--<team>. Returns zero value if the namespace
// does not match the expected pattern.
func ParseNamespace(namespace string) NamespaceInfo {
	parts := nsRE.FindStringSubmatch(namespace)
	if len(parts) != 4 {
		return NamespaceInfo{}
	}
	return NamespaceInfo{
		Environment: parts[1],
		Group:       parts[2],
		Team:        parts[3],
	}
}

// ParseApplicationId parses an applicationId path parameter (<group>--<team>--<appName>)
// and resolves the Kubernetes namespace using the environment from the security context.
func ParseApplicationId(ctx context.Context, applicationId string) (ApplicationIdInfo, error) {
	bCtx, ok := security.FromContext(ctx)
	if !ok {
		return ApplicationIdInfo{}, problems.InternalServerError("Invalid Context", "Security context not found")
	}

	parts := appIdRE.FindStringSubmatch(applicationId)
	if len(parts) != 4 {
		return ApplicationIdInfo{}, problems.BadRequest("Invalid applicationId format, expected <group>--<team>--<appName>")
	}

	return ApplicationIdInfo{
		Environment: bCtx.Environment,
		Group:       parts[1],
		Team:        parts[2],
		AppName:     parts[3],
		Namespace:   bCtx.Environment + "--" + parts[1] + "--" + parts[2],
	}, nil
}

func ParseResourceId(ctx context.Context, resourceId string) (ResourceIdInfo, error) {
	bCtx, ok := security.FromContext(ctx)
	if !ok {
		return ResourceIdInfo{}, problems.InternalServerError("Invalid Context", "Security context not found")
	}

	parts := resourceIdRE.FindStringSubmatch(resourceId)
	if len(parts) != 4 {
		return ResourceIdInfo{}, problems.BadRequest("Invalid resourceId format, expected <group>--<team>--<resourceName>")
	}

	namespace := bCtx.Environment + "--" + parts[1] + "--" + parts[2]
	name := parts[3]

	return ResourceIdInfo{
		Namespace: namespace,
		Name:      name,
	}, nil
}

// MakeResourceId builds a <group>--<team>--<name> string from a Kubernetes object.
func MakeResourceId(obj client.Object) string {
	info := ParseNamespace(obj.GetNamespace())
	if info.Group == "" {
		return obj.GetNamespace() + "--" + obj.GetName()
	}
	return info.Group + "--" + info.Team + "--" + obj.GetName()
}

func MakeResourceName(obj client.Object) string {
	name := obj.GetName()
	if strings.Contains(name, "--") {
		// If the name already contains "--", we assume it's in the format <group>--<team>--<name>
		// and we return only the last part as the resource name
		parts := strings.Split(name, "--")
		return parts[len(parts)-1]
	}
	return name
}

// VerifyApplicationLabel checks that the object carries the expected application
// label. Returns a NotFound problem if the label is missing or does not match,
// so that resources belonging to a different application in the same namespace
// are invisible to the caller.
func VerifyApplicationLabel(obj client.Object, expectedAppName string) error {
	labels := obj.GetLabels()
	if labels == nil || labels[ApplicationLabelKey] != expectedAppName {
		return problems.NotFound(fmt.Sprintf("resource %s/%s not found for application %s",
			obj.GetNamespace(), obj.GetName(), expectedAppName))
	}
	return nil
}

func SplitTeamName(teamName string) (group string, team string) {
	parts := strings.Split(teamName, "--")
	if len(parts) != 2 {
		return "", teamName
	}
	return parts[0], parts[1]
}
