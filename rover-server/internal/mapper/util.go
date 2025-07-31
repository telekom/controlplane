// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// <env>--<group>--<team>
var nsRE = regexp.MustCompile(`^([a-z0-9-]+)--([a-z0-9-]+)--([a-z0-9-]+)$`)

// <namespace>(<group>--<team>)--<resourceName>
var idRE = regexp.MustCompile(`^([a-z0-9-]+--[a-z0-9-]+)--([a-z0-9-]+)$`)

func CopyFromTo[S any, T any](from S, to T) error {
	jsonBytes, err := json.Marshal(from)
	if err != nil {
		return errors.Wrap(err, "failed to marshal")
	}

	err = json.Unmarshal(jsonBytes, to)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal")
	}

	return nil
}

func Len[T any](v *[]T) int {
	if v == nil {
		return 0
	}
	return len(*v)
}

func ToPtr[T any](v T) *T {
	return &v
}

func MakeResourceId(obj client.Object) string {
	parts := nsRE.FindStringSubmatch(obj.GetNamespace())
	if len(parts) == 4 {
		// omit environment prefix
		return parts[2] + "--" + parts[3] + "--" + obj.GetName()
	}
	if len(parts) == 3 {
		return parts[1] + "--" + parts[2] + "--" + obj.GetName()
	}
	return obj.GetNamespace() + "--" + obj.GetName()
}

type ResourceIdInfo struct {
	ResourceId  string
	Environment string
	Namespace   string
	Name        string
}

func ParseResourceId(ctx context.Context, resourceId string) (i ResourceIdInfo, err error) {
	bCtx, ok := security.FromContext(ctx)
	if !ok {
		return i, problems.InternalServerError("Invalid Context", "Security context not found")
	}

	parts := idRE.FindStringSubmatch(resourceId)
	if len(parts) != 3 {
		return i, problems.BadRequest("Invalid resourceId format")
	}

	i = ResourceIdInfo{
		ResourceId:  resourceId,
		Environment: bCtx.Environment,
		Namespace:   parts[1],
		Name:        parts[2],
	}
	return
}
