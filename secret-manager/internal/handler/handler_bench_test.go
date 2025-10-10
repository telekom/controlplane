// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/cyberark/conjur-api-go/conjurapi"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/secret-manager/internal/api"
	"github.com/telekom/controlplane/secret-manager/internal/handler"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/cache"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur"
	"github.com/telekom/controlplane/secret-manager/pkg/controller"
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

	ctrl := controller.NewController(cachedBackend, onboarder)
	// -----------------------------

	handler := api.NewStrictHandler(handler.NewHandler(ctrl), nil)
	app := fiber.New()
	api.RegisterHandlersWithOptions(app, handler, api.FiberServerOptions{
		BaseURL: "/api",
	})

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
			runBenchmarkOnboardTeam(ctx, b, app, env, teamId)
		}
	})

	b.StopTimer()
}

func runBenchmarkOnboardTeam(ctx context.Context, b *testing.B, app *fiber.App, env, teamId string) {
	body := api.TeamWriteRequest{
		Secrets: []api.NamedSecret{
			{
				Name:  "clientSecret",
				Value: "secret",
			},
			{
				Name:  "teamToken",
				Value: "token",
			},
		},
	}
	buf := bytes.NewBuffer(nil)
	err := json.NewEncoder(buf).Encode(body)
	if err != nil {
		b.Fatal(err)
	}
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/onboarding/environments/%s/teams/%s", env, teamId), buf)
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req)
	if err != nil {
		b.Fatal(err)
	}

	if res.StatusCode != 200 {
		ct, _ := io.ReadAll(res.Body)
		b.Fatalf("got status code %d, want 200: %q", res.StatusCode, string(ct))
	}

	var resp api.OnboardingResponseJSONResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		b.Fatal(err)
	}

	assertSecretRefs(b, resp, map[string]string{
		"clientSecret": fmt.Sprintf(`^%s:%s::clientSecret:.+$`, env, teamId),
		"teamToken":    fmt.Sprintf(`^%s:%s::teamToken:.+$`, env, teamId),
	})
}

func assertSecretRefs(b *testing.B, got api.OnboardingResponseJSONResponse, want map[string]string) {
	if len(got.Items) != len(want) {
		b.Fatalf("got %d secret refs, want %d", len(got.Items), len(want))
	}

	for _, item := range got.Items {
		wantRef, ok := want[item.Name]
		if !ok {
			b.Errorf("missing secret ref for key %q", item.Name)
			continue
		}
		if !regexp.MustCompile(wantRef).MatchString(item.Id) {
			b.Errorf("secret ref for key %q: got %q, want to match %q", item.Name, item.Id, wantRef)
		}

		// b.Logf("want=%s, got=%s", wantRef, item.Id)
	}
}
