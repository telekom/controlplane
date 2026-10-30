// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package base

import (
	"path/filepath"
	"slices"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/parser"
)

// FileCommand extends the base Command for file operations
type FileCommand struct {
	*BaseCommand
	FilePath  string
	Recursive bool
	MaxDepth  int
	Parser    *parser.ObjectParser
}

// NewFileCommand creates a new file command with file-related flags
func NewFileCommand(use, short, long string) *FileCommand {
	baseCmd := NewCommand(use, short, long)

	cmd := &FileCommand{
		BaseCommand: baseCmd,
		MaxDepth:    2,
	}

	baseCmd.Cmd.Flags().StringVarP(&cmd.FilePath, "file", "f", "", "Path to the file or directory containing resource definitions")

	return cmd
}

// RecursiveHelp describes directory discovery for commands that support -R/--recursive.
const RecursiveHelp = "By default, a directory passed with -f/--file is processed recursively up to two levels of subdirectories. " +
	"Only .yaml, .yml and .json files (case-insensitive) are used. " +
	"Set --max-depth to change the maximum depth: the input directory has depth 0, its subdirectories depth 1, and so on. " +
	"Files deeper than the limit are ignored. --max-depth=0 includes only files directly inside the input directory; negative values are rejected. " +
	"Use --recursive=false to process only the input directory regardless of the depth limit. " +
	"Symlinked subdirectories are not followed."

// AddRecursiveFlag registers directory traversal flags with recursion enabled
// by default up to two levels below the input directory.
func (c *FileCommand) AddRecursiveFlag() {
	c.Cmd.Flags().BoolVarP(&c.Recursive, "recursive", "R", true,
		"Process the directory used in -f, --file recursively; use --recursive=false to disable")
	c.Cmd.Flags().IntVar(&c.MaxDepth, "max-depth", 2,
		"Maximum subdirectory depth; the directory used in -f, --file has depth 0 (must be non-negative)")
}

// InitParser initializes the parser for processing files
func (c *FileCommand) InitParser() error {
	opts := append(slices.Clone(parser.Opts), parser.WithRecursive(c.Recursive), parser.WithMaxDepth(c.MaxDepth))
	c.Parser = parser.NewObjectParser(opts...)

	c.Logger().V(1).Info("Initialized parser")

	return nil
}

// ParseFiles parses files using the configured parser
func (c *FileCommand) ParseFiles() error {
	if c.MaxDepth < 0 {
		return errors.Errorf("invalid --max-depth %d: must be 0 or greater", c.MaxDepth)
	}

	if c.Parser == nil {
		c.Logger().V(1).Info("Parser not initialized, initializing now")
		if err := c.InitParser(); err != nil {
			return err
		}
	}

	path := resolveFilePath(c.FilePath)
	c.Logger().V(1).Info("Parsing files", "path", path, "recursive", c.Recursive, "maxDepth", c.MaxDepth)

	if err := c.Parser.Parse(path); err != nil {
		return errors.Wrap(err, "failed to parse files")
	}

	c.Logger().V(1).Info("Successfully parsed files", "count", len(c.Parser.Objects()))

	return nil
}

// resolveFilePath joins a nonempty relative path with the configured resource path if set.
// Empty and absolute paths are returned unchanged.
func resolveFilePath(path string) string {
	base := config.ResourcePath()
	if base == "" || path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}
