// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"encoding/json"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
	yaml "github.com/goccy/go-yaml"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/rover-ctl/pkg/cmderrors"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

type HookStage string

const (
	HookAfterParse HookStage = "after_parse"
)

type ObjectParser struct {
	validator *validator.Validate
	objects   []types.Object
	index     int

	hooks map[HookStage]HookFunc
}

type HookFunc func(obj types.Object) error

type Option func(*Options)

type Options struct {
	Validate bool
	Hooks    map[HookStage]HookFunc
}

func EnableValidation() Option {
	return func(opts *Options) {
		opts.Validate = true
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
		objects: make([]types.Object, 0),
		index:   0,
		hooks:   options.Hooks,
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
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return errors.Wrap(err, "failed to read directory "+dirPath)
	}

	filesProcessed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		ext := strings.ToLower(filepath.Ext(fileName))

		if ext == ".yaml" || ext == ".yml" || ext == ".json" {
			filePath := filepath.Join(dirPath, fileName)
			err := p.parseFile(filePath)
			if err != nil {
				return errors.Wrap(err, "failed to parse file "+filePath)
			}
			filesProcessed++
		}
	}

	if filesProcessed == 0 {
		return errors.New("no valid YAML or JSON files found in directory " + dirPath)
	}

	return nil
}

func (p *ObjectParser) parseFile(filePath string) error {
	ext := strings.ToLower(filepath.Ext(filePath))

	file, err := os.OpenFile(filePath, os.O_RDONLY, 0o644)
	if err != nil {
		return errors.Wrap(err, "failed to read file "+filePath)
	}
	defer file.Close()

	switch ext {
	case ".yaml", ".yml":
		return p.parseYAML(file)
	case ".json":
		return p.parseJSON(file)
	default:
		return errors.New("unsupported file extension " + ext)
	}

}

func (p *ObjectParser) parseYAML(r io.Reader) error {
	var decodeOpts []yaml.DecodeOption
	if p.validator != nil {
		decodeOpts = append(decodeOpts, yaml.Strict(), yaml.Validator(p.validator))
	}
	decoder := yaml.NewDecoder(r, decodeOpts...)
	for {
		obj := new(types.UnstructuredObject)
		obj.SetProperty("filename", r.(*os.File).Name())

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

func (p *ObjectParser) parseJSON(r io.Reader) error {
	var obj types.UnstructuredObject
	err := json.NewDecoder(r).Decode(&obj)
	if err != nil {
		return errors.Wrap(err, "failed to parse JSON")
	}

	if p.validator != nil {
		if err := p.validator.Struct(&obj); err != nil {
			return errors.Wrap(err, "object validation failed")
		}
	}

	if err := p.RunHooks(HookAfterParse, &obj); err != nil {
		return errors.Wrap(err, "after parse hook failed")
	}

	p.objects = append(p.objects, &obj)
	return nil
}

func (p *ObjectParser) Iterate() iter.Seq[types.Object] {
	return func(yield func(types.Object) bool) {
		for _, obj := range p.objects {
			if !yield(obj) {
				break
			}
		}
	}
}

// Objects returns the slice of parsed objects
func (p *ObjectParser) Objects() []types.Object {
	return p.objects
}

func (p *ObjectParser) RunHooks(stage HookStage, obj types.Object) error {
	if hook, exists := p.hooks[stage]; exists {
		return hook(obj)
	}
	return nil
}
