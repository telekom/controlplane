// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package command

import (
	"fmt"

	"github.com/telekom/controlplane/tools/e2e-tester/pkg/config"
)

func buildCommandEnv(environment config.Environments, tokenEnvKey string) []string {
	env := make([]string, 0, len(environment.Variables)+1)

	if tokenEnvKey != "" && environment.Token != "" {
		env = append(env, fmt.Sprintf("%s=%s", tokenEnvKey, environment.Token))
	}

	for _, envVar := range environment.Variables {
		env = append(env, fmt.Sprintf("%s=%s", envVar.Name, envVar.Value))
	}

	return env
}
