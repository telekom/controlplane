// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Command lms-issuer is a proof-of-concept Envoy ext_authz gRPC server that
// mints a LastMileSecurity (LMS) JWT per request and returns it as an
// Authorization header for Envoy to inject before forwarding upstream.
//
// This is a PoC: the signing key is a stub generated at startup (not persisted,
// not per-realm). See lms-token-issuing.md for the production plan.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log"
	"net"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
)

const listenAddr = ":9002"

// Metadata namespace + key where jwt_authn publishes the verified consumer
// token payload; must match the operator's emission
// (internal/features/envoy/access_control.go: filterJwtAuthn / jwtPayloadMetadataKey).
const (
	jwtAuthnFilter = "envoy.filters.http.jwt_authn"
	jwtPayloadKey  = "jwt_payload"
)

// issuer implements the Envoy ext_authz Authorization service.
type issuer struct {
	authv3.UnimplementedAuthorizationServer
	key *rsa.PrivateKey // ponytail: stub in-memory key; prod = per-realm key from secret-manager (see plan P0.1)
}

// Check mints a fresh LMS JWT from the incoming request context and returns it
// as an Authorization header on an OK response.
func (s *issuer) Check(ctx context.Context, req *authv3.CheckRequest) (*authv3.CheckResponse, error) {
	attrs := req.GetAttributes()
	http := attrs.GetRequest().GetHttp()

	// Per-route constants (realm, environment) arrive as context_extensions,
	// set by the gateway via ext_authz ExtAuthzPerRoute.CheckSettings.
	ext := attrs.GetContextExtensions()

	// The verified consumer-token claims arrive in metadata_context under the
	// jwt_authn filter namespace, key jwtPayloadMetadataKey. The gateway
	// forwards this via ext_authz metadata_context_namespaces.
	consumer := attrs.GetMetadataContext().
		GetFilterMetadata()[jwtAuthnFilter].
		GetFields()[jwtPayloadKey].GetStructValue().GetFields()

	// Claim sources are context + verified consumer-token claims — no body.
	now := time.Now()
	azp := consumer["azp"].GetStringValue()
	claims := jwt.MapClaims{
		"iss":         "lms-issuer-poc",
		"iat":         now.Unix(),
		"exp":         now.Add(5 * time.Minute).Unix(),
		"requestPath": http.GetPath(),
		"operation":   http.GetMethod(),
		"env":         ext["environment"],
		"realm":       ext["realm"],
		// Derived from the incoming consumer-token's verified claims.
		"sub":      consumer["sub"].GetStringValue(),
		"clientId": consumer["clientId"].GetStringValue(),
		"azp":      azp,
	}

	// CustomScopes: the gateway delivers the per-consumer scope map (and a
	// default) as context_extensions (see internal/features/envoy/
	// last_mile_security.go lmsVhostPerFilterConfig). Select the scope set by
	// the verified azp; fall back to the default when the consumer has none.
	if scope := scopesFor(ext, azp); scope != "" {
		claims["scope"] = scope
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(s.key)
	if err != nil {
		log.Printf("signing failed: %v", err)
		return denied("token signing failed"), nil
	}

	log.Printf("issued LMS token realm=%q env=%q path=%q azp=%q scope=%q", claims["realm"], claims["env"], claims["requestPath"], azp, claims["scope"])

	return &authv3.CheckResponse{
		Status: &status.Status{Code: 0}, // OK
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{
				Headers: []*corev3.HeaderValueOption{{
					Header: &corev3.HeaderValue{
						Key:   "Authorization",
						Value: "Bearer " + signed,
					},
				}},
			},
		},
	}, nil
}

func denied(msg string) *authv3.CheckResponse {
	// Code 7 = PERMISSION_DENIED; fail closed (plan P2.8).
	return &authv3.CheckResponse{Status: &status.Status{Code: 7, Message: msg}}
}

// scopesFor resolves the OAuth2 scope string the LMS token should carry for the
// given consumer (azp). The gateway delivers scopes as context_extensions:
//   - "consumerScopes": a JSON object {consumerName: "space separated scopes"}.
//   - "defaultScopes": applied when the consumer has no explicit entry.
//
// context_extensions is opaque map<string,string>, so consumerScopes is a
// JSON-encoded value. A malformed value is treated as "no per-consumer scopes"
// and falls back to the default (fail-open on config, not on auth).
func scopesFor(ext map[string]string, azp string) string {
	if raw := ext["consumerScopes"]; raw != "" && azp != "" {
		perConsumer := map[string]string{}
		if err := json.Unmarshal([]byte(raw), &perConsumer); err != nil {
			log.Printf("malformed consumerScopes context_extension: %v", err)
		} else if s, ok := perConsumer[azp]; ok {
			return s
		}
	}
	return ext["defaultScopes"]
}

func main() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("generating stub key: %v", err)
	}

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", listenAddr, err)
	}

	srv := grpc.NewServer()
	authv3.RegisterAuthorizationServer(srv, &issuer{key: key})

	log.Printf("LMS issuer (ext_authz PoC) listening on %s", listenAddr)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
