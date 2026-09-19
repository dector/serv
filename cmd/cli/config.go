package main

import (
	"fmt"
	"os"

	"github.com/dector/serv/internal/config"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v3"
)

// effectiveServeConfig maps CLI flags, including deprecated ones, to the
// effective serving configuration.
func effectiveServeConfig(cmd *cli.Command) (config.Serve, error) {
	mode, err := config.ParseMode(cmd.String("mode"))
	if err != nil {
		return config.Serve{}, err
	}

	if cmd.Bool("preview") {
		fmt.Fprintln(os.Stderr, "Warning: --preview is deprecated; use --mode preview instead.")
		if cmd.IsSet("mode") && mode == config.ModeFile {
			return config.Serve{}, errors.New("--preview cannot be combined with --mode file")
		}
		mode = config.ModePreview
	}

	strategy := config.DirResolveReadmeFirst
	if mode == config.ModeFile {
		strategy = config.DirResolveNone
	}
	if cmd.IsSet("dir-resolve") {
		strategy, err = config.ParseDirResolve(cmd.String("dir-resolve"))
		if err != nil {
			return config.Serve{}, err
		}
	}
	if cmd.Bool("no-index-resolve") {
		fmt.Fprintln(os.Stderr, "Warning: --no-index-resolve is deprecated; use --dir-resolve instead.")
		if cmd.IsSet("dir-resolve") {
			fmt.Fprintln(os.Stderr, "Warning: --no-index-resolve ignored because --dir-resolve was provided.")
		} else if mode == config.ModePreview {
			strategy = config.DirResolveReadmeOnly
		}
	}

	return config.Serve{Mode: mode, DirResolve: strategy}, nil
}
