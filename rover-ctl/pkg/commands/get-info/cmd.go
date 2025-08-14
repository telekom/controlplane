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
	*base.FileCommand
	Name     string
	FilePath string
	Shallow  bool
}

// NewCommand creates a new delete command
func NewCommand() *cobra.Command {
	baseCmd := base.NewFileCommand(
		"get-info",
		"Get information about a resource",
		"Get detailed information about a resource using its metadata",
	)
	cmd := &Command{
		FileCommand: baseCmd,
	}

	baseCmd.Cmd.Flags().StringVarP(&cmd.Name, "name", "n", "", "Name of the resource to get information about")
	baseCmd.Cmd.MarkFlagsOneRequired("name", "file")

	baseCmd.Cmd.Flags().BoolVarP(&cmd.Shallow, "shallow", "s", false, "Get only basic information without details")

	cmd.Cmd.RunE = cmd.Run
	cmd.Cmd.PreRunE = func(_ *cobra.Command, args []string) error {
		return cmd.SetupToken()
	}

	return cmd.Cmd
}

func (c *Command) Run(cmd *cobra.Command, args []string) error {
	c.Logger().V(1).Info("Starting get-info command")

	if c.Name != "" {
		return c.getInfoFor(c.Name)
	}

	if c.FilePath != "" {
		if err := c.InitParser(); err != nil {
			return err
		}
		if err := c.ParseFiles(); err != nil {
			return err
		}
		for _, obj := range c.Parser.Objects() {
			if err := c.getInfoFor(obj.GetName()); err != nil {
				return err
			}
		}
	}

	c.Logger().V(1).Info("Completed get-info command")
	return nil
}

func (c *Command) getInfoFor(name string) error {
	roverHandler, err := handlers.GetHandler("Rover", "tcp.ei.telekom.de/v1")
	if err != nil {
		return errors.Wrap(err, "failed to get rover handler")
	}

	c.Logger().V(1).Info("Getting info for resource", "name", name)

	info, err := roverHandler.Info(c.Cmd.Context(), name)
	if err != nil {
		return c.HandleError(err, "get info for Rover")
	}

	prettyString, err := util.FormatOutput(info, viper.GetString("output.format"))
	if err != nil {
		return errors.Wrap(err, "failed to format output")
	}

	_, err = c.Cmd.OutOrStdout().Write([]byte(prettyString))
	if err != nil {
		return errors.Wrap(err, "failed to write output")
	}

	c.Logger().V(1).Info("Successfully retrieved info for resource", "name", name)

	return nil
}
