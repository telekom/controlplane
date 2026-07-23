// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package publication

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/telekom/controlplane/gateway/internal/features/envoy"
	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"
)

const (
	compilerVersion = "envoy-poc-v1"
	envoyVersion    = "1.37"
)

// BundleFromResources converts the compiler result into the versioned publication envelope.
func BundleFromResources(resources *envoy.ResourceBundle) (*xdsapi.Bundle, error) {
	if resources == nil {
		return nil, fmt.Errorf("resource bundle is required")
	}
	resources.Sort()
	bundle := &xdsapi.Bundle{
		TargetId:            targetID(resources.Target),
		PublisherGeneration: publisherGeneration(resources.Source),
		SchemaVersion:       xdsapi.SchemaVersion,
		CompilerVersion:     compilerVersion,
		EnvoyVersion:        envoyVersion,
	}
	if bundle.TargetId == "" {
		return nil, fmt.Errorf("resource bundle target is required")
	}
	for _, source := range resources.Source.Resources {
		bundle.Sources = append(bundle.Sources, &xdsapi.SourceReference{
			Kind: source.Kind, Namespace: source.Namespace, Name: source.Name,
			Uid: string(source.UID), Generation: source.Generation,
		})
	}
	var err error
	if bundle.Listeners, err = packResources(resources.Listeners); err != nil {
		return nil, fmt.Errorf("packing listeners: %w", err)
	}
	if bundle.Routes, err = packResources(resources.Routes); err != nil {
		return nil, fmt.Errorf("packing routes: %w", err)
	}
	if bundle.Clusters, err = packResources(resources.Clusters); err != nil {
		return nil, fmt.Errorf("packing clusters: %w", err)
	}
	if bundle.Endpoints, err = packResources(resources.Endpoints); err != nil {
		return nil, fmt.Errorf("packing endpoints: %w", err)
	}
	if err := xdsapi.SetDigest(bundle); err != nil {
		return nil, fmt.Errorf("digesting bundle: %w", err)
	}
	return bundle, nil
}

func targetID(target envoy.TargetIdentity) string {
	parts := []string{target.Environment, target.Namespace, target.Name, string(target.UID)}
	for _, part := range parts {
		if strings.Contains(part, "/") {
			return ""
		}
	}
	if target.Environment == "" || target.Namespace == "" || target.Name == "" || target.UID == "" {
		return ""
	}
	return strings.Join(parts, "/")
}

func publisherGeneration(source envoy.SourceMetadata) string {
	content := strings.Builder{}
	content.WriteString(xdsapi.SchemaVersion)
	content.WriteByte(0)
	content.WriteString(compilerVersion)
	content.WriteByte(0)
	content.WriteString(envoyVersion)
	content.WriteByte('\n')
	for _, reference := range source.Resources {
		content.WriteString(reference.Kind)
		content.WriteByte(0)
		content.WriteString(reference.Namespace)
		content.WriteByte(0)
		content.WriteString(reference.Name)
		content.WriteByte(0)
		content.WriteString(string(reference.UID))
		content.WriteByte(0)
		content.WriteString(strconv.FormatInt(reference.Generation, 10))
		content.WriteByte('\n')
	}
	digest := sha256.Sum256([]byte(content.String()))
	return hex.EncodeToString(digest[:])
}

func packResources[T proto.Message](resources []T) ([]*anypb.Any, error) {
	packed := make([]*anypb.Any, 0, len(resources))
	for _, resource := range resources {
		value, err := anypb.New(resource)
		if err != nil {
			return nil, fmt.Errorf("packing resource: %w", err)
		}
		value.Value, err = (proto.MarshalOptions{Deterministic: true}).Marshal(resource)
		if err != nil {
			return nil, fmt.Errorf("canonically packing resource: %w", err)
		}
		packed = append(packed, value)
	}
	return packed, nil
}
