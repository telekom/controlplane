// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
)

var (
	environment string
	group       string
	team        string
	rawScopes   string
	scopes      []string
)

func init() {
	flag.StringVar(&environment, "env", "poc", "Environment")
	flag.StringVar(&group, "group", "eni", "Group")
	flag.StringVar(&team, "team", "hyperion", "Team")
	flag.StringVar(&rawScopes, "scopes", "", "Scopes")
}

func main() {
	flag.Parse()
	scopes = strings.Split(rawScopes, ",")
	fmt.Printf("Creating token for environment=%s, group=%s, team=%s, scopes=%v\n", environment, group, team, scopes)
	fmt.Printf("`%s`\n", mock.NewMockAccessToken(environment, group, team, scopes))
}
