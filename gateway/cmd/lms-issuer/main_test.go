// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	"crypto/rand"
	"crypto/rsa"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/protobuf/types/known/structpb"
)

// TestCheckIssuesVerifiableToken is the one runnable check: a Check call must
// return an OK response with an Authorization header holding a JWT that
// verifies against the issuer key and carries the request-derived claims.
func TestCheckIssuesVerifiableToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s := &issuer{key: key}

	req := &authv3.CheckRequest{
		Attributes: &authv3.AttributeContext{
			ContextExtensions: map[string]string{
				"environment": "prod",
				"realm":       "realm1",
			},
			MetadataContext: &corev3.Metadata{
				FilterMetadata: map[string]*structpb.Struct{
					jwtAuthnFilter: {Fields: map[string]*structpb.Value{
						jwtPayloadKey: structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"sub":      structpb.NewStringValue("user-uuid"),
								"clientId": structpb.NewStringValue("client-x"),
								"azp":      structpb.NewStringValue("client-x"),
							},
						}),
					}},
				},
			},
			Request: &authv3.AttributeContext_Request{
				Http: &authv3.AttributeContext_HttpRequest{
					Method:  "GET",
					Path:    "/api/v1/things",
					Headers: map[string]string{},
				},
			},
		},
	}

	resp, err := s.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if resp.GetStatus().GetCode() != 0 {
		t.Fatalf("expected OK (0), got %d: %s", resp.GetStatus().GetCode(), resp.GetStatus().GetMessage())
	}

	var authHeader string
	for _, h := range resp.GetOkResponse().GetHeaders() {
		if h.GetHeader().GetKey() == "Authorization" {
			authHeader = h.GetHeader().GetValue()
		}
	}
	if authHeader == "" {
		t.Fatal("no Authorization header returned")
	}

	const prefix = "Bearer "
	if len(authHeader) <= len(prefix) || authHeader[:len(prefix)] != prefix {
		t.Fatalf("Authorization header not a Bearer token: %q", authHeader)
	}
	raw := authHeader[len(prefix):]

	tok, err := jwt.Parse(raw, func(*jwt.Token) (interface{}, error) { return &key.PublicKey, nil })
	if err != nil || !tok.Valid {
		t.Fatalf("minted token failed verification: %v", err)
	}

	claims := tok.Claims.(jwt.MapClaims)
	if claims["realm"] != "realm1" || claims["env"] != "prod" || claims["requestPath"] != "/api/v1/things" {
		t.Fatalf("claims not derived from request: %#v", claims)
	}
	if claims["sub"] != "user-uuid" || claims["clientId"] != "client-x" || claims["azp"] != "client-x" {
		t.Fatalf("consumer claims not derived from jwt metadata: %#v", claims)
	}
}
