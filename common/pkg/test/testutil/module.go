// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"fmt"
	"go/build"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/mod/modfile"
)

var (
	PkgModCrdsSubpath = "config/crd/bases"
	IgnoreOwnModPath  = true
)

// goPathOrDefault returns the GOPATH environment variable or the default GOPATH
func goPathOrDefault() string {
	goPath := os.Getenv("GOPATH")
	if goPath == "" {
		goPath = build.Default.GOPATH
	}

	return goPath
}

// GetCrdPathsOrDie same as LoadCrdPaths but panics on error
func GetCrdPathsOrDie(modPathRE string) []string {
	paths, err := GetCrdPaths(modPathRE)
	if err != nil {
		panic(err)
	}

	return paths
}

// GetCrdPaths will detect matching module paths from go.mod and return the CRD paths
// The go.mod file is detected by traversing the directory tree starting from the current working directory
func GetCrdPaths(modPathRE string) (paths []string, err error) {
	goModPath, err := findNextFileMatch("go.mod")
	if err != nil {
		return nil, errors.Wrap(err, "failed to find go.mod")
	}

	return getCrdPaths(goModPath, modPathRE)
}

func getCrdPaths(goModFilePath, modPathRE string) (paths []string, err error) {
	content, err := os.ReadFile(goModFilePath)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to read '%s'", goModFilePath))
	}

	modFile, err := modfile.Parse("go.mod", content, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse go.mod")
	}

	re, err := regexp.Compile(modPathRE)
	if err != nil {
		return nil, errors.Wrap(err, "invalid regex")
	}

	ownModPath := modFile.Module.Mod.Path
	goPath := goPathOrDefault()

	for _, req := range modFile.Require {
		if IgnoreOwnModPath && strings.HasPrefix(req.Mod.Path, ownModPath) {
			continue
		}

		if !re.MatchString(req.Mod.Path) {
			continue
		}

		if isReplaced, newPath := findReplacePath(modFile, req.Mod.Path); isReplaced {
			log.Printf("Module %s has been replaced. Trying relative path %s", req.Mod.Path, newPath)
			crdFilepath := filepath.Join(newPath, PkgModCrdsSubpath)
			paths = append(paths, crdFilepath)
			continue
		}

		crdFilepath := filepath.Join(goPath, "pkg/mod", fmt.Sprintf("%s@%s", req.Mod.Path, req.Mod.Version), PkgModCrdsSubpath)
		paths = append(paths, crdFilepath)
	}
	return paths, nil
}

// findNextFileMatch will traverse the directory tree starting from the current working directory
// and return the relative path to the first file that matches the given filename
func findNextFileMatch(filename string) (string, error) {
	startingWd, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(err, "failed to get working directory")
	}

	wd := startingWd
	for {
		if wd == "/" {
			return "", errors.New(filename + " not found")
		}

		if _, err := os.Stat(filepath.Join(wd, filename)); err == nil {
			return filepath.Rel(startingWd, filepath.Join(wd, filename))
		}

		wd = filepath.Dir(wd)
	}
}

func findReplacePath(modFile *modfile.File, modPath string) (bool, string) {
	for _, rep := range modFile.Replace {
		if rep.Old.Path == modPath {
			return true, rep.New.Path
		}
	}

	return false, ""
}
