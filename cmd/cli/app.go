package main

import (
	"fmt"

	"github.com/dector/serv/internal/config"
	"github.com/dector/serv/internal/tailscale"
	"github.com/dector/serv/internal/version"
	"github.com/urfave/cli/v3"
)

// newApp builds the serv CLI command with its flags and arguments.
func newApp() *cli.Command {
	return &cli.Command{
		Name:  "serv",
		Usage: "Serve static files",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Value:   fmt.Sprintf("%d", version.DefaultPort),
				Usage:   "HTTP server port",
			},
			&cli.BoolFlag{
				Name:    "version",
				Aliases: []string{"v"},
				Value:   false,
				Usage:   "Print version and exit",
			},
			&cli.StringFlag{
				Name:    "mode",
				Aliases: []string{"m"},
				Value:   string(config.ModePreview),
				Usage:   "Serving mode: preview/p or file/f",
			},
			&cli.StringFlag{
				Name:  "dir-resolve",
				Usage: "Directory resolution: readme-first/rf, index-first/if, readme-only/ro, index-only/io, none/n",
			},
			&cli.BoolFlag{
				Name:   "no-index-resolve",
				Value:  false,
				Hidden: true,
				Usage:  "Deprecated: use --dir-resolve instead",
			},
			&cli.BoolFlag{
				Name:    "preview",
				Aliases: []string{"P"},
				Value:   false,
				Hidden:  true,
				Usage:   "Deprecated: use --mode preview instead",
			},
			&cli.BoolFlag{
				Name:    "open",
				Aliases: []string{"o"},
				Value:   false,
				Usage:   "Open the served URL in the default browser",
			},
			&cli.StringFlag{
				Name:        "expose-tailscale",
				Aliases:     []string{"T"},
				Value:       tailscale.DefaultExposeValue,
				DefaultText: "local port",
				HideDefault: true,
				Usage:       "Expose via Tailscale Serve; optional value sets HTTPS port",
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Value: false,
				Usage: "Print verbose command output",
			},
			&cli.BoolFlag{
				Name:    "browser",
				Aliases: []string{"B"},
				Value:   false,
				Hidden:  true,
				Usage:   "Deprecated: use --open/-o instead",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "file",
				UsageText: "path to file or directory to serve",
			},
		},
		Action: serveAction,
	}
}
