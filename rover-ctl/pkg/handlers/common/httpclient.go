// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

var NewAuthorizedHttpClient = func(ctx context.Context, tokenUrl, clientId, clientSecret string) HttpDoer {
	baseClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 100,
		},
		Timeout: 10 * time.Second,
	}

	tokenCfg := clientcredentials.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		TokenURL:     tokenUrl,
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, baseClient)
	return tokenCfg.Client(ctx)
}
