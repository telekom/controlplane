// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/util"
)

// ResourceCommandOptions contains options specific to resource commands
type ResourceCommandOptions struct {
	Kind       string
	ApiVersion string
	Name       string
}

// Command represents the resource command
type Command struct {
	*base.BaseCommand
	Options *ResourceCommandOptions
}

// NewCommand creates a new resource command
func NewCommand() *cobra.Command {
	baseCmd := base.NewCommand(
		"resource",
		"Manage resources",
		"Get or list resources from the server",
	)

	resOpts := &ResourceCommandOptions{}

	cmd := &Command{
		BaseCommand: baseCmd,
		Options:     resOpts,
	}

	getCmd := cmd.newGetCommand()
	listCmd := cmd.newListCommand()

	// Add subcommands
	cmd.Cmd.AddCommand(
		getCmd,
		listCmd,
	)

	return cmd.Cmd
}

// newGetCommand creates a command for getting a single resource
func (c *Command) newGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a resource by name",
		Long:  "Get a resource from the server by its kind, api version, and name",
		RunE:  c.runGet,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := base.SetupTokenInContext(cmd.Context())
			if err != nil {
				return errors.Wrap(err, "failed to set up token in context")
			}
			cmd.SetContext(ctx)
			return nil
		},
	}

	cmd.Flags().StringVar(&c.Options.Kind, "kind", "", "Resource kind")
	cmd.Flags().StringVar(&c.Options.ApiVersion, "api-version", "", "API version")
	cmd.MarkFlagRequired("kind")
	cmd.MarkFlagRequired("api-version")

	cmd.Flags().StringVar(&c.Options.Name, "name", "", "Name of the resource to get")
	cmd.MarkFlagRequired("name")

	return cmd
}

// newListCommand creates a command for listing resources
func (c *Command) newListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all resources of a kind",
		Long:  "List all resources of specified kind and api version",
		RunE:  c.runList,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := base.SetupTokenInContext(cmd.Context())
			if err != nil {
				return errors.Wrap(err, "failed to set up token in context")
			}
			cmd.SetContext(ctx)
			return nil
		},
	}

	cmd.Flags().StringVar(&c.Options.Kind, "kind", "", "Resource kind")
	cmd.Flags().StringVar(&c.Options.ApiVersion, "api-version", "", "API version")
	cmd.MarkFlagRequired("kind")
	cmd.MarkFlagRequired("api-version")

	return cmd
}

// runGet executes the get command
func (c *Command) runGet(cmd *cobra.Command, args []string) error {
	c.Logger().V(1).Info("Starting get command",
		"kind", c.Options.Kind,
		"apiVersion", c.Options.ApiVersion,
		"name", c.Options.Name)

	handler, err := handlers.GetHandler(c.Options.Kind, c.Options.ApiVersion)
	if err != nil {
		return errors.Wrapf(err, "no handler found for %s/%s",
			c.Options.ApiVersion, c.Options.Kind)
	}

	c.Logger().V(1).Info("Getting resource", "name", c.Options.Name)
	obj, err := handler.Get(cmd.Context(), c.Options.Name)
	if err != nil {
		return c.HandleError(err, "get resource")
	}

	format := viper.GetString("output.format")
	c.Logger().V(1).Info("Formatting output", "format", format)
	output, err := util.FormatOutput(obj, format)
	if err != nil {
		return errors.Wrap(err, "failed to format output")
	}

	c.Logger().V(0).Info("Successfully retrieved resource",
		"kind", c.Options.Kind,
		"name", c.Options.Name)

	cmd.Println(output)

	return nil
}

// runList executes the list command
func (c *Command) runList(cmd *cobra.Command, args []string) error {
	c.Logger().V(1).Info("Starting list command",
		"kind", c.Options.Kind,
		"apiVersion", c.Options.ApiVersion)

	handler, err := handlers.GetHandler(c.Options.Kind, c.Options.ApiVersion)
	if err != nil {
		c.Logger().Error(err, "Handler not found",
			"kind", c.Options.Kind,
			"apiVersion", c.Options.ApiVersion)
		return errors.Wrapf(err, "no handler found for %s/%s",
			c.Options.ApiVersion, c.Options.Kind)
	}

	c.Logger().V(1).Info("Listing resources")
	objects, err := handler.List(cmd.Context())
	if err != nil {
		return c.HandleError(err, "list resources")
	}

	format := viper.GetString("output.format")
	c.Logger().V(1).Info("Formatting output", "format", format)
	output, err := util.FormatOutput(objects, format)
	if err != nil {
		return errors.Wrap(err, "failed to format output")
	}

	c.Logger().V(0).Info("Successfully listed resources", "kind", c.Options.Kind)

	cmd.Println(output)

	return nil
}
