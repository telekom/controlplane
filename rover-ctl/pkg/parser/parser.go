// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"encoding/json"
	stderrors "errors"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
	yaml "github.com/goccy/go-yaml"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/rover-ctl/pkg/cmderrors"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type HookStage string

const (
	HookAfterParse HookStage = "after_parse"
)

type ObjectParser struct {
	validator *validator.Validate
	objects   []types.Object
	index     int
	recursive bool

	hooks map[HookStage]HookFunc
}

type HookFunc func(obj types.Object) error

type Option func(*Options)

type Options struct {
	Validate  bool
	Recursive bool
	Hooks     map[HookStage]HookFunc
}

func EnableValidation() Option {
	return func(opts *Options) {
		opts.Validate = true
	}
}

// WithRecursive enables recursive discovery of supported files in nested
// directories when a directory is parsed. Symlinked directories are not followed.
func WithRecursive(recursive bool) Option {
	return func(opts *Options) {
		opts.Recursive = recursive
	}
}

func WithHook(stage HookStage, hook HookFunc) Option {
	return func(opts *Options) {
		if opts.Hooks == nil {
			opts.Hooks = make(map[HookStage]HookFunc)
		}
		opts.Hooks[stage] = hook
	}
}

func NewObjectParser(opts ...Option) *ObjectParser {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	op := &ObjectParser{
		objects:   make([]types.Object, 0),
		index:     0,
		hooks:     options.Hooks,
		recursive: options.Recursive,
	}
	if options.Validate {
		op.validator = validator.New()
	}

	return op
}

func (p *ObjectParser) Parse(path string) error {
	if path == "" {
		return errors.New("path cannot be empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return cmderrors.FileNotFound(path)
	}

	if info.IsDir() {
		return p.parseDirectory(path)
	}

	return p.parseFile(path)
}

func (p *ObjectParser) parseDirectory(dirPath string) error {
	filesProcessed, err := p.walkDirectory(dirPath)
	if err != nil {
		return err
	}

	if filesProcessed == 0 {
		return errors.New("no valid YAML or JSON files found in directory " + dirPath)
	}

	return nil
}

// walkDirectory parses supported files in dirPath in lexicographic order and,
// if recursive parsing is enabled, descends depth-first into subdirectories.
// It returns the number of supported files processed.
func (p *ObjectParser) walkDirectory(dirPath string) (int, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read directory "+dirPath)
	}

	filesProcessed := 0
	for _, entry := range entries {
		entryPath := filepath.Join(dirPath, entry.Name())

		if entry.IsDir() {
			if !p.recursive {
				continue
			}
			count, err := p.walkDirectory(entryPath)
			if err != nil {
				return filesProcessed, err
			}
			filesProcessed += count
			continue
		}

		if !isSupportedFile(entry.Name()) {
			continue
		}

		if err := p.parseFile(entryPath); err != nil {
			return filesProcessed, errors.Wrap(err, "failed to parse file "+entryPath)
		}
		filesProcessed++
	}

	return filesProcessed, nil
}

func isSupportedFile(fileName string) bool {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

func (p *ObjectParser) parseFile(filePath string) error {
	ext := strings.ToLower(filepath.Ext(filePath))

	file, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to read file "+filePath)
	}

	decodedReader := transform.NewReader(
		file,
		unicode.BOMOverride(unicode.UTF8.NewDecoder()),
	)
	decodedContent, decodeErr := io.ReadAll(decodedReader)
	closeErr := file.Close()
	if err := stderrors.Join(
		errors.Wrap(decodeErr, "failed to decode content in "+filePath),
		errors.Wrap(closeErr, "failed to close file "+filePath),
	); err != nil {
		return err
	}

	content, err := SubstitutePlaceholders(string(decodedContent))
	if err != nil {
		return errors.Wrap(err, "failed to substitute placeholders in "+filePath)
	}

	reader := strings.NewReader(content)

	switch ext {
	case ".yaml", ".yml":
		return p.parseYAML(reader, filePath)
	case ".json":
		return p.parseJSON(reader, filePath)
	default:
		return errors.New("unsupported file extension " + ext)
	}
}

func (p *ObjectParser) parseYAML(r io.Reader, filename string) error {
	var decodeOpts []yaml.DecodeOption
	if p.validator != nil {
		decodeOpts = append(decodeOpts, yaml.Strict(), yaml.Validator(p.validator))
	}
	decoder := yaml.NewDecoder(r, decodeOpts...)
	for {
		obj := new(types.UnstructuredObject)
		obj.SetProperty("filename", filename)

		err := decoder.Decode(obj)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return errors.Wrap(err, "failed to parse YAML")
		}

		if err := p.RunHooks(HookAfterParse, obj); err != nil {
			return errors.Wrap(err, "after parse hook failed")
		}

		p.objects = append(p.objects, obj)
	}
	return nil

}

func (p *ObjectParser) parseJSON(r io.Reader, filename string) error {
	content := make(map[string]any)
	err := json.NewDecoder(r).Decode(&content)
	if err != nil {
		return errors.Wrap(err, "failed to parse JSON")
	}

	obj := &types.UnstructuredObject{
		Content: content,
	}
	obj.SetProperty("filename", filename)

	if p.validator != nil {
		if err := p.validator.Struct(obj); err != nil {
			return errors.Wrap(err, "object validation failed")
		}
	}

	if err := p.RunHooks(HookAfterParse, obj); err != nil {
		return errors.Wrap(err, "after parse hook failed")
	}

	p.objects = append(p.objects, obj)
	return nil
}

func (p *ObjectParser) Iterate() iter.Seq[types.Object] {
	return func(yield func(types.Object) bool) {
		for _, obj := range removeUnknownObjects(p.objects) {
			if !yield(obj) {
				break
			}
		}
	}
}

// Objects returns the slice of parsed objects
func (p *ObjectParser) Objects() []types.Object {
	return removeUnknownObjects(p.objects)
}

func (p *ObjectParser) RunHooks(stage HookStage, obj types.Object) error {
	if hook, exists := p.hooks[stage]; exists {
		return hook(obj)
	}
	return nil
}

func removeUnknownObjects(objects []types.Object) []types.Object {
	var filtered []types.Object
	for _, obj := range objects {
		if obj.GetKind() != "" && obj.GetApiVersion() != "" {
			filtered = append(filtered, obj)
		} else {
			log.L().V(1).Info("Skipping unknown object", "filename", obj.GetProperty("filename"), "name", obj.GetName())
		}
	}
	return filtered
}

func FilterByKindAndVersion(objects []types.Object, kind, apiVersion string) []types.Object {
	var filtered []types.Object
	for _, obj := range objects {
		if strings.EqualFold(obj.GetKind(), kind) && strings.EqualFold(obj.GetApiVersion(), apiVersion) {
			filtered = append(filtered, obj)
		}
	}
	return filtered
}
