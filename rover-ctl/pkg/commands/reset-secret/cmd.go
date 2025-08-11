// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resetsecret

import (
	"github.com/spf13/cobra"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
)

type Command struct {
	*base.BaseCommand
	Name string
}

// NewCommand creates a new delete command
func NewCommand() *cobra.Command {
	baseCmd := base.NewCommand(
		"reset-secret",
		"Reset a secret",
		"Reset a secret for an application",
	)
	cmd := &Command{
		BaseCommand: baseCmd,
	}

	baseCmd.Cmd.Flags().StringVarP(&cmd.Name, "application", "a", "", "Name of the application to reset the secret for")
	baseCmd.Cmd.MarkFlagRequired("name")

	cmd.Cmd.RunE = cmd.Run
	cmd.Cmd.PreRunE = func(_ *cobra.Command, args []string) error {
		return cmd.SetupToken()
	}

	return cmd.Cmd
}

func (c *Command) Run(cmd *cobra.Command, args []string) error {

	// TODO: Implement the logic to reset the secret for the specified application

	return nil
}
