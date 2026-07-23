// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"strings"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"

	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"

	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
)

// ValidationErrors represents all independently discoverable bundle errors.
type ValidationErrors []*xdsapi.ValidationError

var supportedNestedTypes = map[protoreflect.FullName]map[string]struct{}{
	"envoy.config.listener.v3.Filter.typed_config": {
		"type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager": {},
	},
	"envoy.extensions.filters.network.http_connection_manager.v3.HttpFilter.typed_config": {
		"type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz":          {},
		"type.googleapis.com/envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication": {},
		"type.googleapis.com/envoy.extensions.filters.http.rbac.v3.RBAC":                   {},
		"type.googleapis.com/envoy.extensions.filters.http.router.v3.Router":               {},
	},
	"envoy.config.route.v3.VirtualHost.typed_per_filter_config": {
		"type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthzPerRoute": {},
		"type.googleapis.com/envoy.extensions.filters.http.jwt_authn.v3.PerRouteConfig":   {},
		"type.googleapis.com/envoy.extensions.filters.http.rbac.v3.RBACPerRoute":          {},
	},
	"envoy.config.route.v3.Route.typed_per_filter_config": {
		"type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthzPerRoute": {},
		"type.googleapis.com/envoy.extensions.filters.http.jwt_authn.v3.PerRouteConfig":   {},
		"type.googleapis.com/envoy.extensions.filters.http.rbac.v3.RBACPerRoute":          {},
	},
	"envoy.config.core.v3.TransportSocket.typed_config": {
		"type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext": {},
	},
	"envoy.config.cluster.v3.Cluster.typed_extension_protocol_options": {
		"type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {},
	},
}

func (e ValidationErrors) Error() string {
	return fmt.Sprintf("bundle validation failed with %d error(s)", len(e))
}

// Validate verifies the envelope and returns a consistent go-control-plane snapshot.
func Validate(bundle *xdsapi.Bundle, version string) (*cachev3.Snapshot, ValidationErrors) {
	if bundle == nil {
		return nil, ValidationErrors{validationError(
			xdsapi.ValidationCode_VALIDATION_CODE_REQUIRED, "bundle", "bundle is required")}
	}
	validationErrors := validateEnvelope(bundle)

	resources, resourceErrors := validateResources(bundle)
	validationErrors = append(validationErrors, resourceErrors...)
	if len(validationErrors) > 0 {
		return nil, validationErrors
	}

	snapshot, err := cachev3.NewSnapshot(version, resources)
	if err != nil {
		return nil, ValidationErrors{validationError(
			xdsapi.ValidationCode_VALIDATION_CODE_MALFORMED_RESOURCE, "resources", "snapshot cannot be created")}
	}
	if err := snapshot.Consistent(); err != nil {
		return nil, ValidationErrors{validationError(
			xdsapi.ValidationCode_VALIDATION_CODE_INCONSISTENT_SNAPSHOT,
			"resources", "snapshot references are inconsistent")}
	}
	return snapshot, nil
}

func validateEnvelope(bundle *xdsapi.Bundle) ValidationErrors {
	var validationErrors ValidationErrors
	if bundle.TargetId == "" {
		validationErrors = append(validationErrors, validationError(
			xdsapi.ValidationCode_VALIDATION_CODE_REQUIRED, "target_id", "target ID is required"))
	}
	if bundle.PublisherGeneration == "" {
		validationErrors = append(validationErrors, validationError(
			xdsapi.ValidationCode_VALIDATION_CODE_REQUIRED,
			"publisher_generation", "publisher generation is required"))
	}
	if bundle.SchemaVersion != xdsapi.SchemaVersion {
		validationErrors = append(validationErrors, validationError(
			xdsapi.ValidationCode_VALIDATION_CODE_UNSUPPORTED_TYPE,
			"schema_version", "unsupported bundle schema version"))
	}
	if bundle.CompilerVersion == "" {
		validationErrors = append(validationErrors, validationError(
			xdsapi.ValidationCode_VALIDATION_CODE_REQUIRED, "compiler_version", "compiler version is required"))
	}
	if bundle.EnvoyVersion != "1.37" {
		validationErrors = append(validationErrors, validationError(
			xdsapi.ValidationCode_VALIDATION_CODE_UNSUPPORTED_TYPE, "envoy_version", "unsupported Envoy version"))
	}
	if len(bundle.Sources) == 0 {
		validationErrors = append(validationErrors, validationError(
			xdsapi.ValidationCode_VALIDATION_CODE_REQUIRED, "sources", "at least one source is required"))
	}
	digest, err := xdsapi.Digest(bundle)
	if err != nil || bundle.Digest == "" || digest != bundle.Digest {
		validationErrors = append(validationErrors, validationError(
			xdsapi.ValidationCode_VALIDATION_CODE_DIGEST_MISMATCH, "digest", "canonical digest does not match"))
	}
	return validationErrors
}

func validateResources(bundle *xdsapi.Bundle) (map[resource.Type][]cachetypes.Resource, ValidationErrors) {
	var validationErrors ValidationErrors
	resources := map[resource.Type][]cachetypes.Resource{}
	sets := []struct {
		field   string
		typeURL resource.Type
		values  []*anypb.Any
		new     func() cachetypes.Resource
	}{
		{"listeners", resource.ListenerType, bundle.Listeners, func() cachetypes.Resource { return &listenerv3.Listener{} }},
		{"routes", resource.RouteType, bundle.Routes, func() cachetypes.Resource { return &routev3.RouteConfiguration{} }},
		{"clusters", resource.ClusterType, bundle.Clusters, func() cachetypes.Resource { return &clusterv3.Cluster{} }},
		{"endpoints", resource.EndpointType, bundle.Endpoints, func() cachetypes.Resource { return &endpointv3.ClusterLoadAssignment{} }},
	}
	for _, set := range sets {
		resources[set.typeURL] = []cachetypes.Resource{}
		seen := make(map[string]struct{}, len(set.values))
		for index, value := range set.values {
			field := fmt.Sprintf("%s[%d]", set.field, index)
			if value == nil || value.TypeUrl != set.typeURL {
				validationErrors = append(validationErrors, validationError(
					xdsapi.ValidationCode_VALIDATION_CODE_UNSUPPORTED_TYPE, field, "unexpected protobuf type URL"))
				continue
			}
			message := set.new()
			if unmarshalErr := anypb.UnmarshalTo(value, message, proto.UnmarshalOptions{}); unmarshalErr != nil {
				validationErrors = append(validationErrors, validationError(
					xdsapi.ValidationCode_VALIDATION_CODE_MALFORMED_RESOURCE, field, "resource cannot be decoded"))
				continue
			}
			if validator, ok := message.(interface{ ValidateAll() error }); ok {
				if validationErr := validator.ValidateAll(); validationErr != nil {
					validationErrors = append(validationErrors, validationError(
						xdsapi.ValidationCode_VALIDATION_CODE_MALFORMED_RESOURCE,
						field, "resource violates Envoy validation constraints"))
					continue
				}
			}
			if validationErr := validateNestedMessages(message.ProtoReflect()); validationErr != nil {
				validationErrors = append(validationErrors, validationError(
					xdsapi.ValidationCode_VALIDATION_CODE_MALFORMED_RESOURCE,
					field, "nested xDS extension violates Envoy validation constraints"))
				continue
			}
			if containsSecretBearingField(message.ProtoReflect()) {
				validationErrors = append(validationErrors, validationError(
					xdsapi.ValidationCode_VALIDATION_CODE_UNSUPPORTED_TYPE,
					field, "secret-bearing xDS configuration is not supported"))
				continue
			}
			name := cachev3.GetResourceName(message)
			if name == "" {
				validationErrors = append(validationErrors, validationError(
					xdsapi.ValidationCode_VALIDATION_CODE_REQUIRED, field, "resource name is required"))
				continue
			}
			if _, exists := seen[name]; exists {
				validationErrors = append(validationErrors, validationError(
					xdsapi.ValidationCode_VALIDATION_CODE_DUPLICATE_RESOURCE, field, "duplicate resource name"))
				continue
			}
			seen[name] = struct{}{}
			resources[set.typeURL] = append(resources[set.typeURL], message)
		}
	}
	return resources, validationErrors
}

func containsSecretBearingField(message protoreflect.Message) bool {
	if !message.IsValid() {
		return false
	}
	if typed, ok := message.Interface().(*anypb.Any); ok {
		unpacked, err := anypb.UnmarshalNew(typed, proto.UnmarshalOptions{})
		return err != nil || containsSecretBearingField(unpacked.ProtoReflect())
	}
	secret := false
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		name := string(field.Name())
		if strings.Contains(name, "private_key") || strings.Contains(name, "password") ||
			strings.Contains(name, "secret") || strings.Contains(name, "ticket_key") {
			secret = true
			return false
		}
		switch {
		case field.IsList() && field.Kind() == protoreflect.MessageKind:
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				if containsSecretBearingField(list.Get(i).Message()) {
					secret = true
					return false
				}
			}
		case field.IsMap() && field.MapValue().Kind() == protoreflect.MessageKind:
			value.Map().Range(func(_ protoreflect.MapKey, mapValue protoreflect.Value) bool {
				if containsSecretBearingField(mapValue.Message()) {
					secret = true
					return false
				}
				return true
			})
		case field.Kind() == protoreflect.MessageKind && containsSecretBearingField(value.Message()):
			secret = true
			return false
		}
		return !secret
	})
	return secret
}

func validateNestedMessages(message protoreflect.Message) error {
	if !message.IsValid() {
		return nil
	}
	var validationErr error
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch {
		case field.IsList() && field.Kind() == protoreflect.MessageKind:
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				if err := validateNestedField(field, list.Get(i).Message()); err != nil {
					validationErr = err
					return false
				}
			}
		case field.IsMap() && field.MapValue().Kind() == protoreflect.MessageKind:
			value.Map().Range(func(_ protoreflect.MapKey, mapValue protoreflect.Value) bool {
				if err := validateNestedField(field, mapValue.Message()); err != nil {
					validationErr = err
					return false
				}
				return true
			})
		case field.Kind() == protoreflect.MessageKind:
			validationErr = validateNestedField(field, value.Message())
		}
		return validationErr == nil
	})
	return validationErr
}

func validateNestedField(field protoreflect.FieldDescriptor, message protoreflect.Message) error {
	typed, ok := message.Interface().(*anypb.Any)
	if !ok {
		return validateNestedMessages(message)
	}
	allowed, supportedField := supportedNestedTypes[field.FullName()]
	if !supportedField {
		return fmt.Errorf("unsupported nested Any field %q", field.FullName())
	}
	if _, supportedType := allowed[typed.TypeUrl]; !supportedType {
		return fmt.Errorf("unsupported nested Any type for field %q", field.FullName())
	}
	unpacked, err := anypb.UnmarshalNew(typed, proto.UnmarshalOptions{})
	if err != nil {
		return fmt.Errorf("unpacking nested xDS extension: %w", err)
	}
	if validator, ok := unpacked.(interface{ ValidateAll() error }); ok {
		if err := validator.ValidateAll(); err != nil {
			return fmt.Errorf("validating nested xDS extension: %w", err)
		}
	}
	return validateNestedMessages(unpacked.ProtoReflect())
}

func validationError(code xdsapi.ValidationCode, field, message string) *xdsapi.ValidationError {
	return &xdsapi.ValidationError{Code: code, Field: field, Message: message}
}
