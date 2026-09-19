// Package config holds the serving mode and directory resolution options.
package config

import (
	"strings"

	"github.com/pkg/errors"
)

// Mode selects how files are served.
type Mode string

// DirResolve selects how a directory request is resolved to a file.
type DirResolve string

// Serve is the effective runtime configuration.
type Serve struct {
	Mode       Mode
	DirResolve DirResolve
}

const (
	ModePreview Mode = "preview"
	ModeFile    Mode = "file"

	DirResolveReadmeFirst DirResolve = "readme-first"
	DirResolveIndexFirst  DirResolve = "index-first"
	DirResolveReadmeOnly  DirResolve = "readme-only"
	DirResolveIndexOnly   DirResolve = "index-only"
	DirResolveNone        DirResolve = "none"
)

// ParseMode maps user input to a serving mode.
func ParseMode(value string) (Mode, error) {
	switch strings.ToLower(value) {
	case "preview", "p", "":
		return ModePreview, nil
	case "file", "f":
		return ModeFile, nil
	default:
		return "", errors.Errorf("invalid mode %q (expected preview/p or file/f)", value)
	}
}

// ParseDirResolve maps user input to a directory resolution strategy.
func ParseDirResolve(value string) (DirResolve, error) {
	switch strings.ToLower(value) {
	case "readme-first", "rf", "":
		return DirResolveReadmeFirst, nil
	case "index-first", "if":
		return DirResolveIndexFirst, nil
	case "readme-only", "ro":
		return DirResolveReadmeOnly, nil
	case "index-only", "io":
		return DirResolveIndexOnly, nil
	case "none", "n":
		return DirResolveNone, nil
	default:
		return "", errors.Errorf("invalid dir-resolve %q", value)
	}
}
