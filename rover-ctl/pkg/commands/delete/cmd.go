// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package delete

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/base"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// Command represents the delete command
type Command struct {
	*base.FileCommand
}

// NewCommand creates a new delete command
func NewCommand() *cobra.Command {
	cmd := &Command{
		FileCommand: base.NewFileCommand(
			"delete",
			"Delete a resource",
			"Delete a resource defined in a file or directory from the server",
		),
	}

	cmd.Cmd.MarkFlagRequired("file")

	// Set the run function
	cmd.Cmd.RunE = cmd.Run
	cmd.Cmd.PreRunE = func(_ *cobra.Command, args []string) error {
		return cmd.SetupToken()
	}
	return cmd.Cmd
}

// Run executes the delete command
func (c *Command) Run(cmd *cobra.Command, args []string) error {
	c.Logger().V(1).Info("Starting delete command")

	if err := c.ParseFiles(); err != nil {
		return err
	}

	// Process objects
	count := 0

	for _, obj := range handlers.Sort(c.Parser.Objects()) {
		c.Logger().V(1).Info("Processing object", "kind", obj.GetKind(), "name", obj.GetName())

		if err := c.deleteObject(obj); err != nil {
			c.Logger().Error(err, "Failed to delete object", "kind", obj.GetKind(), "name", obj.GetName())
			return errors.Wrapf(err, "failed to delete object %s", obj.GetName())
		}
		count++
	}

	// Print summary
	c.Logger().V(0).Info("Successfully deleted resources", "count", count)
	cmd.Printf("Successfully deleted %d resource(s)\n", count)
	return nil
}

// deleteObject processes a single object from the parser
func (c *Command) deleteObject(obj types.Object) error {
	// Get the appropriate handler based on the object kind and API version
	handler, err := handlers.GetHandler(obj.GetKind(), obj.GetApiVersion())
	if err != nil {
		if errors.Is(err, handlers.ErrNoHandlerFound) {
			c.Logger().Info("Handler not found. Ignoring...",
				"kind", obj.GetKind(),
				"apiVersion", obj.GetApiVersion())
			return nil
		}
		return err
	}

	c.Logger().Info(fmt.Sprintf("🧹 Deleting %s",
		obj.GetKind()),
		"apiVersion", obj.GetApiVersion(),
		"name", obj.GetName())

	// Delete the object using the handler
	if err := handler.Delete(c.Cmd.Context(), obj); err != nil {
		return c.HandleError(err, fmt.Sprintf("delete %s", obj.GetKind()))
	}

	status, err := handler.WaitForDeleted(c.Cmd.Context(), obj.GetName())
	if err != nil {
		return c.HandleError(err, fmt.Sprintf("wait for %s to be deleted", obj.GetKind()))
	}

	if status.IsGone() {
		c.Logger().Info(fmt.Sprintf("✅ Successfully deleted %s",
			obj.GetKind()),
			"name", obj.GetName())
	}

	return nil
}
