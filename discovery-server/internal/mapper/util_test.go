// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

type parseIDResult struct {
	namespace string
	name      string
}

func runParseIDCases(
	t *testing.T,
	tests []struct {
		name      string
		id        string
		expectErr bool
		expect    parseIDResult
	},
	parseFn func(context.Context, string) (parseIDResult, error),
) {
	t.Helper()

	ctx := security.ToContext(context.Background(), &security.BusinessContext{Environment: "poc"})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFn(ctx, tt.id)
			if tt.expectErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != tt.expect {
				t.Fatalf("expected %#v, got %#v", tt.expect, got)
			}
		})
	}
}

func TestParseNamespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		namespace string
		expect    NamespaceInfo
	}{
		{
			name:      "valid namespace",
			namespace: "poc--eni--hyperion",
			expect: NamespaceInfo{
				Environment: "poc",
				Group:       "eni",
				Team:        "hyperion",
			},
		},
		{
			name:      "invalid namespace",
			namespace: "invalid",
			expect:    NamespaceInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseNamespace(tt.namespace)
			if got != tt.expect {
				t.Fatalf("expected %#v, got %#v", tt.expect, got)
			}
		})
	}
}

func TestParseIds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		missingContext string
		parseFn        func(context.Context, string) (parseIDResult, error)
		validID        string
		expectedName   string
		invalidID      string
	}{
		{
			name:           "application id",
			missingContext: "eni--hyperion--my-app",
			validID:        "eni--hyperion--my-app",
			expectedName:   "my-app",
			invalidID:      "invalid",
			parseFn: func(ctx context.Context, id string) (parseIDResult, error) {
				got, err := ParseApplicationId(ctx, id)
				if err != nil {
					return parseIDResult{}, err
				}
				return parseIDResult{namespace: got.Namespace, name: got.AppName}, nil
			},
		},
		{
			name:           "resource id",
			missingContext: "eni--hyperion--resource",
			validID:        "eni--hyperion--resource",
			expectedName:   "resource",
			invalidID:      "invalid",
			parseFn: func(ctx context.Context, id string) (parseIDResult, error) {
				got, err := ParseResourceId(ctx, id)
				if err != nil {
					return parseIDResult{}, err
				}
				return parseIDResult{namespace: got.Namespace, name: got.Name}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("missing security context", func(t *testing.T) {
				_, err := tt.parseFn(context.Background(), tt.missingContext)
				if err == nil {
					t.Fatal("expected error when security context is missing")
				}
			})

			runParseIDCases(t, []struct {
				name      string
				id        string
				expectErr bool
				expect    parseIDResult
			}{
				{
					name:      "valid id",
					id:        tt.validID,
					expectErr: false,
					expect: parseIDResult{
						namespace: "poc--eni--hyperion",
						name:      tt.expectedName,
					},
				},
				{
					name:      "invalid id",
					id:        tt.invalidID,
					expectErr: true,
				},
			}, tt.parseFn)
		})
	}
}

func TestMakeResourceIdAndName(t *testing.T) {
	idObj := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "poc--eni--hyperion",
		},
	}

	id := MakeResourceId(idObj)
	if id != "eni--hyperion--my-app" {
		t.Fatalf("unexpected resource id: %q", id)
	}

	nameObj := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "eni--hyperion--my-app",
			Namespace: "poc--eni--hyperion",
		},
	}

	name := MakeResourceName(nameObj)
	if name != "my-app" {
		t.Fatalf("unexpected resource name: %q", name)
	}

	invalidNSObj := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "invalid",
		},
	}
	if got := MakeResourceId(invalidNSObj); got != "invalid--my-app" {
		t.Fatalf("unexpected resource id for invalid namespace: %q", got)
	}

	plainNameObj := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "poc--eni--hyperion",
		},
	}
	if got := MakeResourceName(plainNameObj); got != "my-app" {
		t.Fatalf("unexpected plain resource name: %q", got)
	}

	// ApiExposure k8s names follow the pattern <appName>--<exposureName>.
	// MakeResourceName must strip the app prefix and return only the exposure name.
	exposureObj := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app--eni-distr-v1",
			Namespace: "poc--eni--hyperion",
		},
	}
	if got := MakeResourceName(exposureObj); got != "eni-distr-v1" {
		t.Fatalf("unexpected exposure resource name: got %q, want %q", got, "eni-distr-v1")
	}
}

func TestVerifyApplicationLabel(t *testing.T) {
	obj := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "poc--eni--hyperion",
			Labels: map[string]string{
				ApplicationLabelKey: "my-app",
			},
		},
	}

	if err := VerifyApplicationLabel(obj, "my-app"); err != nil {
		t.Fatalf("expected label verification to pass, got %v", err)
	}

	if err := VerifyApplicationLabel(obj, "other-app"); err == nil {
		t.Fatal("expected label verification to fail for mismatched app")
	}
}

func TestSplitTeamName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		expectGrp string
		expectTm  string
	}{
		{
			name:      "group and team",
			input:     "eni--hyperion",
			expectGrp: "eni",
			expectTm:  "hyperion",
		},
		{
			name:      "plain team",
			input:     "hyperion",
			expectGrp: "",
			expectTm:  "hyperion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group, team := SplitTeamName(tt.input)
			if group != tt.expectGrp || team != tt.expectTm {
				t.Fatalf("unexpected split: group=%q team=%q", group, team)
			}
		})
	}
}
