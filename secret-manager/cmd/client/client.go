// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/accesstoken"
	"go.uber.org/zap"
)

var (
	url       string
	token     string
	tokenFile string

	secretID    string
	secretValue string
	rotate      bool

	envId  string
	teamId string
	appId  string
	delete bool

	secretsApi    api.SecretsApi
	onboardingApi api.OnboardingApi
)

func init() {
	flag.StringVar(&url, "url", "", "API URL")
	flag.StringVar(&token, "token", "", "API Token")
	flag.StringVar(&tokenFile, "token-file", "", "API Token File")

	flag.StringVar(&secretID, "id", "", "Secret ID")
	flag.StringVar(&secretValue, "value", "", "Secret Value")
	flag.BoolVar(&rotate, "rotate", false, "Rotate Secret")

	flag.StringVar(&envId, "env", "", "Environment ID")
	flag.StringVar(&teamId, "team", "", "Team ID")
	flag.StringVar(&appId, "app", "", "Application ID")
	flag.BoolVar(&delete, "delete", false, "Delete Secret")
}

func main() {
	flag.Parse()

	log := zapr.NewLogger(zap.Must(zap.NewDevelopment()))
	ctx := logr.NewContext(context.Background(), log)

	opts := []api.Option{}
	if url != "" {
		opts = append(opts, api.WithURL(url))
	}
	if token != "" {
		opts = append(opts, api.WithAccessToken(accesstoken.NewStaticAccessToken(token)))
	} else if tokenFile != "" {
		opts = append(opts, api.WithAccessToken(accesstoken.NewAccessToken(tokenFile)))
	}

	onboardingApi = api.NewOnboarding(opts...)
	secretsApi = api.NewSecrets(opts...)

	if secretID != "" && secretValue == "" {
		res, err := secretsApi.Get(ctx, secretID)
		if err != nil {
			panic(err)
		}
		fmt.Println("Secret Value:", res)
		return
	}

	if secretID != "" && rotate {
		res, err := secretsApi.Rotate(ctx, secretID)
		if err != nil {
			panic(err)
		}
		fmt.Println("Rotated Secret ID:", res)
		return
	}

	if secretID != "" && secretValue != "" {
		res, err := secretsApi.Set(ctx, secretID, secretValue)
		if err != nil {
			panic(err)
		}
		fmt.Println("New Secret ID:", res)
		return
	}

	if envId != "" && teamId != "" && appId != "" {
		onboardOrDeleteApp(ctx, onboardingApi, envId, teamId, appId)
		return
	}
	if envId != "" && teamId != "" {
		onboardOrDeleteTeam(ctx, onboardingApi, envId, teamId)
		return
	}
	if envId != "" {
		onboardOrDeleteEnv(ctx, onboardingApi, envId)
		return
	}
}

func onboardOrDeleteEnv(ctx context.Context, onboardingApi api.OnboardingApi, envId string) {
	if delete {
		err := onboardingApi.DeleteEnvironment(ctx, envId)
		if err != nil {
			panic(err)
		}
		fmt.Println("Deleted Environment ID:", envId)
	} else {
		res, err := onboardingApi.UpsertEnvironment(ctx, envId)
		if err != nil {
			panic(err)
		}
		fmt.Println("Upserted Environment ID:", res)
		listAvailableSecrets(ctx, res)
	}
}

func onboardOrDeleteTeam(ctx context.Context, onboardingApi api.OnboardingApi, envId, teamId string) {
	if delete {
		err := onboardingApi.DeleteTeam(ctx, envId, teamId)
		if err != nil {
			panic(err)
		}
		fmt.Println("Deleted Team ID:", teamId)
	} else {
		res, err := onboardingApi.UpsertTeam(ctx, envId, teamId)
		if err != nil {
			panic(err)
		}
		fmt.Println("Upserted Team ID:", res)
		listAvailableSecrets(ctx, res)
	}
}

func onboardOrDeleteApp(ctx context.Context, onboardingApi api.OnboardingApi, envId, teamId, appId string) {
	if delete {
		err := onboardingApi.DeleteApplication(ctx, envId, teamId, appId)
		if err != nil {
			panic(err)
		}
		fmt.Println("Deleted Application ID:", appId)
	} else {
		res, err := onboardingApi.UpsertApplication(ctx, envId, teamId, appId)
		if err != nil {
			panic(err)
		}
		fmt.Println("Upserted Application ID:", res)
		listAvailableSecrets(ctx, res)
	}
}

func listAvailableSecrets(ctx context.Context, availableSecrets map[string]string) {
	for secretName, secretId := range availableSecrets {
		res, err := secretsApi.Get(ctx, secretId)
		if err != nil {
			panic(err)
		}

		fmt.Printf("%s='%s'\n", secretName, res)
	}
}
