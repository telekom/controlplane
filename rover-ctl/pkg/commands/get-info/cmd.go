// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package getinfo

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/util"
)

type Command struct {
	*base.BaseCommand
	Name string
}

// NewCommand creates a new delete command
func NewCommand() *cobra.Command {
	baseCmd := base.NewCommand(
		"get-info",
		"Get information about a resource",
		"Get detailed information about a resource using its metadata",
	)
	cmd := &Command{
		BaseCommand: baseCmd,
	}

	// Add file-specific flags
	baseCmd.Cmd.Flags().StringVarP(&cmd.Name, "name", "n", "", "Name of the resource to get information about")

	cmd.Cmd.RunE = cmd.Run
	cmd.Cmd.PreRunE = func(_ *cobra.Command, args []string) error {
		return cmd.SetupToken()
	}

	return cmd.Cmd
}

func (c *Command) Run(cmd *cobra.Command, args []string) error {
	roverHandler, err := handlers.GetHandler("Rover", "tcp.ei.telekom.de/v1")
	if err != nil {
		return errors.Wrap(err, "failed to get rover handler")
	}

	c.Logger.V(1).Info("Getting info for resource", "name", c.Name)

	info, err := roverHandler.Info(cmd.Context(), c.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get info for resource %s", c.Name)
	}

	prettyString, err := util.FormatOutput(info, viper.GetString("output.format"))
	if err != nil {
		return errors.Wrap(err, "failed to format output")
	}

	_, err = c.Cmd.OutOrStdout().Write([]byte(prettyString))
	if err != nil {
		return errors.Wrap(err, "failed to write output")
	}

	c.Logger.V(1).Info("Successfully retrieved info for resource", "name", c.Name)

	return nil
}
