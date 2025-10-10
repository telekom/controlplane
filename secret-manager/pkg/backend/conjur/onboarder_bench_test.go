// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/cyberark/conjur-api-go/conjurapi"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/cache"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur"
	"github.com/telekom/controlplane/secret-manager/test/mocks"
)

func BenchmarkOnboardTeam(b *testing.B) {
	mockWriter := mocks.NewMockConjurAPI(b)
	mockReader := mocks.NewMockConjurAPI(b)

	conjur.RootPolicyPath = "controlplane"
	backend := conjur.NewBackend(mockWriter, mockReader)
	// cachedBackend := v2.NewCachedBackend(backend, 5*time.Second)
	cachedBackend := cache.NewCachedBackend(backend, 5*time.Second)
	onboarder := conjur.NewOnboarder(mockWriter, cachedBackend)

	ctx := context.Background()

	env := "dev"

	// Set expectations for policy loading
	mockWriter.EXPECT().LoadPolicy(conjurapi.PolicyModePost, "controlplane/"+env, mock.Anything).Return(nil, nil)

	maxTeams := 20
	for i := range maxTeams {
		teamId := fmt.Sprintf("team%d", i)
		mockReader.EXPECT().RetrieveSecret(fmt.Sprintf("controlplane/%s/%s/clientSecret", env, teamId)).Return([]byte("secret"), nil).Maybe()
		mockReader.EXPECT().RetrieveSecret(fmt.Sprintf("controlplane/%s/%s/teamToken", env, teamId)).Return([]byte("token"), nil).Maybe()
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		goroutineNum := int(time.Now().UnixNano() % int64(maxTeams))
		teamId := fmt.Sprintf("team%d", goroutineNum)

		for pb.Next() {
			runBenchmarkOnboardTeam(ctx, b, onboarder, env, teamId)
		}
	})

	b.StopTimer()
}

func runBenchmarkOnboardTeam(ctx context.Context, b *testing.B, onboarder *conjur.ConjurOnboarder, env, teamId string) {
	resp, err := onboarder.OnboardTeam(ctx, env, teamId,
		backend.WithSecretValue("clientSecret", backend.String("secret")),
		backend.WithSecretValue("teamToken", backend.String("token")),
	)
	if err != nil {
		b.Fatal(err)
	}

	assertSecretRefs(b, resp.SecretRefs(), map[string]string{
		"clientSecret": fmt.Sprintf(`^%s:%s::clientSecret:.+$`, env, teamId),
		"teamToken":    fmt.Sprintf(`^%s:%s::teamToken:.+$`, env, teamId),
	})
}

func assertSecretRefs(b *testing.B, got map[string]backend.SecretRef, want map[string]string) {
	if len(got) != len(want) {
		b.Fatalf("got %d secret refs, want %d", len(got), len(want))
	}
	for key, wantRef := range want {
		gotRef, ok := got[key]
		if !ok {
			b.Errorf("missing secret ref for key %q", key)
			continue
		}
		if !regexp.MustCompile(wantRef).MatchString(gotRef.String()) {
			b.Errorf("secret ref for key %q: got %q, want to match %q", key, gotRef.String(), wantRef)
		}

		b.Logf("want=%s, got=%s", wantRef, gotRef.String())
	}
}
