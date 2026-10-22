// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package getinfo

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/parser"
	"github.com/telekom/controlplane/rover-ctl/pkg/util"
)

const (
	// Currently get-info is only supported by Rover resources
	apiVersion = "tcp.ei.telekom.de/v1"
	kind       = "Rover"
)

type Command struct {
	*base.FileCommand
	Name    string
	Shallow bool
}

// NewCommand creates a new get-info command
func NewCommand() *cobra.Command {
	baseCmd := base.NewFileCommand(
		"get-info",
		"Get information about a resource",
		`Get detailed information about a specific resource by name or multiple resources from the server.
If no name or file is provided, information about all resources of the specified type will be retrieved.`,
	)
	cmd := &Command{
		FileCommand: baseCmd,
	}

	cmd.Cmd.Flags().StringVarP(&cmd.Name, "name", "n", "", "Name of the application to get information about")
	cmd.Cmd.Flags().StringVarP(&cmd.Name, "application", "a", "", "Name of the application to get information about (alias for --name)")
	cmd.Cmd.MarkFlagsMutuallyExclusive("name", "application", "file")

	cmd.Cmd.Flags().BoolVarP(&cmd.Shallow, "shallow", "s", false, "Get only information about the application itself without sub-resources")

	cmd.Cmd.RunE = cmd.Run

	return cmd.Cmd
}

func (c *Command) Run(cmd *cobra.Command, args []string) error {
	c.Logger().V(1).Info("Starting get-info command", "name", c.Name, "file", c.FilePath, "shallow", c.Shallow)

	if c.nameFlagChanged() && strings.TrimSpace(c.Name) == "" {
		return errors.New("application name must not be empty")
	}

	if c.Shallow {
		cmd.SetContext(common.WithShallowInfo(cmd.Context()))
	}

	if c.Name != "" {
		return base.ForEach(c.BaseCommand, "get info for", []string{c.Name}, c.getInfoFor)
	}

	if c.FilePath != "" {
		if err := c.InitParser(); err != nil {
			return err
		}
		if err := c.ParseFiles(); err != nil {
			return err
		}

		roverObjects := parser.FilterByKindAndVersion(c.Parser.Objects(), kind, apiVersion)
		if len(roverObjects) == 0 {
			return errors.New("no Rover resources found in the provided file(s)")
		}
		c.Logger().V(1).Info("Filtered Rover resources from parsed objects", "total", len(c.Parser.Objects()), "roverCount", len(roverObjects))
		names := make([]string, 0, len(roverObjects))
		for _, obj := range roverObjects {
			names = append(names, obj.GetName())
		}
		return base.ForEach(c.BaseCommand, "get info for", names, c.getInfoFor)
	}

	if c.Name == "" && c.FilePath == "" {
		return c.getInfoMany()
	}

	c.Logger().V(1).Info("Completed get-info command")
	return nil
}

func (c *Command) nameFlagChanged() bool {
	return c.Cmd.Flags().Changed("name") || c.Cmd.Flags().Changed("application")
}

func (c *Command) getInfoMany() error {
	roverHandler, err := handlers.GetHandler(kind, apiVersion)
	if err != nil {
		return errors.Wrap(err, "failed to get rover handler")
	}

	c.Logger().V(1).Info("Getting info for multiple resources")

	infoList, err := roverHandler.InfoMany(c.Cmd.Context(), nil)
	if err != nil {
		return c.HandleError(err, "get info for Rovers")
	}

	prettyString, err := util.FormatOutput(infoList, viper.GetString("output.format"))
	if err != nil {
		return errors.Wrap(err, "failed to format output")
	}

	_, err = c.Cmd.OutOrStdout().Write([]byte(prettyString))
	if err != nil {
		return errors.Wrap(err, "failed to write output")
	}

	c.Logger().V(1).Info("Successfully retrieved info for multiple resources")

	return nil
}

func (c *Command) getInfoFor(name string) error {
	roverHandler, err := handlers.GetHandler(kind, apiVersion)
	if err != nil {
		return errors.Wrap(err, "failed to get rover handler")
	}

	c.Logger().V(1).Info("Getting info for resource", "name", name)

	info, err := roverHandler.Info(c.Cmd.Context(), name)
	if err != nil {
		return errors.Wrapf(err, "Rover %q", name)
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
