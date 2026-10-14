// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
	"github.com/telekom/controlplane/rover-ctl/pkg/util"
)

var defaultApiVersion = "tcp.ei.telekom.de/v1"

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
		// Markdown-friendly: "~~~" fences avoid backticks inside the raw string;
		// inline code spans are concatenated.
		`Get, list or delete resources on the server using the subcommands:

~~~
roverctl resource list --kind KIND
roverctl resource get --kind KIND --name NAME
roverctl resource delete --kind KIND --name NAME
~~~

Deprecated: the selector flags `+"`--list`, `--get` and `--delete`"+` on this command are
still supported for backward compatibility, but print a deprecation notice to
stderr. Use the matching subcommand instead. The legacy single-dash form
`+"`-name=NAME`"+` is accepted as an alias for `+"`--name=NAME`"+`.`,
	)

	resOpts := &ResourceCommandOptions{}

	cmd := &Command{
		BaseCommand: baseCmd,
		Options:     resOpts,
	}

	cmd.setupLegacyFlags()

	// Add subcommands
	cmd.Cmd.AddCommand(
		cmd.newGetCommand(),
		cmd.newListCommand(),
		cmd.newDeleteCommand(),
	)

	return cmd.Cmd
}

// setupLegacyFlags configures the selector flags (--list, --get, --delete) on the resource command itself.
func (c *Command) setupLegacyFlags() {
	c.Cmd.Args = cobra.NoArgs
	// Authentication happens in runLegacy after validation, so that invalid or
	// missing selectors are reported without requiring a token.
	c.Cmd.PreRunE = nil
	c.Cmd.RunE = c.runLegacy

	flags := c.Cmd.Flags()
	flags.StringVar(&c.Options.Kind, "kind", "", "Resource kind")
	flags.StringVar(&c.Options.ApiVersion, "api-version", defaultApiVersion, "API version")
	flags.StringVarP(&c.Options.Name, "name", "n", "", "Name of the resource (required with --get and --delete, ignored with --list)")
	flags.Bool(selectorList, false, `Deprecated: use "roverctl resource list" instead`)
	flags.Bool(selectorGet, false, `Deprecated: use "roverctl resource get" instead`)
	flags.Bool(selectorDelete, false, `Deprecated: use "roverctl resource delete" instead`)
}

const (
	selectorList   = "list"
	selectorGet    = "get"
	selectorDelete = "delete"
)

// runLegacy dispatches the deprecated selector flags to the matching operation.
func (c *Command) runLegacy(cmd *cobra.Command, args []string) error {
	if cmd.Flags().NFlag() == 0 {
		return cmd.Help()
	}

	var selected []string
	for _, name := range []string{selectorList, selectorGet, selectorDelete} {
		value, err := cmd.Flags().GetBool(name)
		if err != nil {
			return errors.Wrapf(err, "failed to read flag --%s", name)
		}
		if value {
			selected = append(selected, name)
		}
	}

	if len(selected) == 0 {
		return errors.New("exactly one of --list, --get or --delete must be specified")
	}
	if len(selected) > 1 {
		return errors.Errorf("flags --list, --get and --delete are mutually exclusive, got --%s", strings.Join(selected, ", --"))
	}

	selector := selected[0]
	// Printed to stderr only, so JSON/YAML output on stdout stays parseable.
	fmt.Fprintf(cmd.ErrOrStderr(), "Flag \"--%s\" is deprecated, use \"roverctl resource %s\" instead.\n", selector, selector)

	if err := c.Options.validate(selector != selectorList); err != nil {
		return err
	}

	base.PrintBanner(cmd)
	if err := c.SetupToken(); err != nil {
		return errors.Wrap(err, "failed to set up token in context")
	}

	switch selector {
	case selectorGet:
		return c.runGet(cmd, args)
	case selectorDelete:
		return c.runDelete(cmd, args)
	default:
		return c.runList(cmd, args)
	}
}

// validate rejects missing or empty selector values.
func (o *ResourceCommandOptions) validate(requireName bool) error {
	if strings.TrimSpace(o.Kind) == "" {
		return errors.New("--kind must not be empty")
	}
	if strings.TrimSpace(o.ApiVersion) == "" {
		return errors.New("--api-version must not be empty")
	}
	if requireName && strings.TrimSpace(o.Name) == "" {
		return errors.New("--name must not be empty")
	}
	return nil
}

// setupPreRun returns a PreRunE that validates the options and sets up authentication.
func (c *Command) setupPreRun(requireName bool) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := c.Options.validate(requireName); err != nil {
			return err
		}
		base.PrintBanner(cmd)
		ctx, err := base.SetupTokenInContext(cmd.Context())
		if err != nil {
			return errors.Wrap(err, "failed to set up token in context")
		}
		cmd.SetContext(ctx)
		return nil
	}
}

// newGetCommand creates a command for getting a single resource
func (c *Command) newGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get a resource by name",
		Long:    "Get a resource from the server by its kind, api version, and name",
		Args:    cobra.NoArgs,
		RunE:    c.runGet,
		PreRunE: c.setupPreRun(true),
	}

	cmd.Flags().StringVar(&c.Options.Kind, "kind", "", "Resource kind")
	cmd.Flags().StringVar(&c.Options.ApiVersion, "api-version", defaultApiVersion, "API version")
	cmd.MarkFlagRequired("kind")

	cmd.Flags().StringVarP(&c.Options.Name, "name", "n", "", "Name of the resource to get")
	cmd.MarkFlagRequired("name")

	return cmd
}

// newListCommand creates a command for listing resources
func (c *Command) newListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List all resources of a kind",
		Long:    "List all resources of specified kind and api version",
		Args:    cobra.NoArgs,
		RunE:    c.runList,
		PreRunE: c.setupPreRun(false),
	}

	cmd.Flags().StringVar(&c.Options.Kind, "kind", "", "Resource kind")
	cmd.Flags().StringVar(&c.Options.ApiVersion, "api-version", defaultApiVersion, "API version")
	cmd.MarkFlagRequired("kind")

	return cmd
}

// newDeleteCommand creates a command for deleting a single resource
func (c *Command) newDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete a resource by name",
		Long:    "Delete a resource from the server by its kind, api version, and name and wait until it is gone",
		Args:    cobra.NoArgs,
		RunE:    c.runDelete,
		PreRunE: c.setupPreRun(true),
	}

	cmd.Flags().StringVar(&c.Options.Kind, "kind", "", "Resource kind")
	cmd.Flags().StringVar(&c.Options.ApiVersion, "api-version", defaultApiVersion, "API version")
	cmd.MarkFlagRequired("kind")

	cmd.Flags().StringVarP(&c.Options.Name, "name", "n", "", "Name of the resource to delete")
	cmd.MarkFlagRequired("name")

	return cmd
}

// runDelete executes the delete command
func (c *Command) runDelete(cmd *cobra.Command, args []string) error {
	c.Logger().V(1).Info("Starting delete command",
		"kind", c.Options.Kind,
		"apiVersion", c.Options.ApiVersion,
		"name", c.Options.Name)

	handler, err := handlers.GetHandler(c.Options.Kind, c.Options.ApiVersion)
	if err != nil {
		return errors.Wrapf(err, "cannot delete %s with apiVersion %s", c.Options.Kind, c.Options.ApiVersion)
	}

	deleter, ok := handler.(handlers.NameDeleter)
	if !ok {
		return errors.Errorf("deleting %s by name is not supported", c.Options.Kind)
	}

	c.Logger().Info(fmt.Sprintf("🧹 Deleting %s", c.Options.Kind),
		"apiVersion", c.Options.ApiVersion,
		"name", c.Options.Name)

	// A single explicitly requested deletion always fails on error, independent of --fail-fast.
	if err := deleter.DeleteByName(cmd.Context(), c.Options.Name); err != nil {
		return errors.Wrapf(err, "failed to delete %s %q", c.Options.Kind, c.Options.Name)
	}

	status, err := handler.WaitForDeleted(cmd.Context(), c.Options.Name)
	if err != nil {
		if status != nil {
			obj := &types.UnstructuredObject{Content: map[string]any{
				"apiVersion": c.Options.ApiVersion,
				"kind":       c.Options.Kind,
				"metadata":   map[string]any{"name": c.Options.Name},
			}}
			_ = common.NewStatusEval(obj, status).PrettyPrint(cmd.OutOrStdout(), viper.GetString("log.format"))
		}
		return errors.Wrapf(err, "failed to wait for %s %q to be deleted", c.Options.Kind, c.Options.Name)
	}

	if status != nil && status.IsGone() {
		c.Logger().Info(fmt.Sprintf("✅ Successfully deleted %s", c.Options.Kind), "name", c.Options.Name)
	}

	return nil
}

// runGet executes the get command
func (c *Command) runGet(cmd *cobra.Command, args []string) error {
	c.Logger().V(1).Info("Starting get command",
		"kind", c.Options.Kind,
		"apiVersion", c.Options.ApiVersion,
		"name", c.Options.Name)

	handler, err := handlers.GetHandler(c.Options.Kind, c.Options.ApiVersion)
	if err != nil {
		if errors.Is(err, handlers.ErrNoHandlerFound) {
			c.Logger().Info("Handler not found. Ignoring...",
				"kind", c.Options.Kind,
				"apiVersion", c.Options.ApiVersion)
			return nil
		}
		return err
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
		if errors.Is(err, handlers.ErrNoHandlerFound) {
			c.Logger().Info("Handler not found. Ignoring...",
				"kind", c.Options.Kind,
				"apiVersion", c.Options.ApiVersion)
			return nil
		}
		return err
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
