package main

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dector/serv/internal/listen"
	"github.com/dector/serv/internal/output"
	"github.com/dector/serv/internal/server"
	"github.com/dector/serv/internal/tailscale"
	"github.com/dector/serv/internal/version"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v3"
)

func serveAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Bool("version") {
		fmt.Println(version.Version)
		return nil
	}

	cfg, err := effectiveServeConfig(cmd)
	if err != nil {
		return err
	}

	rootFile := cmd.StringArg("file")
	rootFile, err = filepath.Abs(rootFile)
	if err != nil {
		return errors.Wrap(err, "failed to get absolute path")
	}

	rootInfo, err := os.Stat(rootFile)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.Errorf("file does not exist: %s", rootFile)
		}
		return errors.Wrap(err, "failed to stat root file")
	}

	exposeTailscale := cmd.IsSet("expose-tailscale")
	listenConfig, err := selectListenConfig(cmd, rootFile, exposeTailscale)
	if err != nil {
		return err
	}
	tailscaleConfig, err := tailscale.ParseExposeConfig(exposeTailscale, cmd.String("expose-tailscale"), listenConfig.Port.Str)
	if err != nil {
		return err
	}
	if tailscaleConfig.Enabled {
		if err := tailscale.CheckReady(ctx, tailscale.RealRunner{}, tailscaleConfig.Port); err != nil {
			return err
		}
	}

	// Create filesystem rooted at the specified directory or file's parent.
	var fsys fs.FS
	var basePath string
	if rootInfo.IsDir() {
		fsys = os.DirFS(rootFile)
		basePath = "."
	} else {
		fsys = os.DirFS(filepath.Dir(rootFile))
		basePath = filepath.Base(rootFile)
	}

	if cmd.Bool("browser") {
		fmt.Fprintln(os.Stderr, "Warning: --browser/-B is deprecated and will be removed in a future release. Use --open/-o instead.")
	}

	handler := server.NewHandler(server.Options{
		FS:       fsys,
		BasePath: basePath,
		RootFile: rootFile,
		RootInfo: rootInfo,
		Config:   cfg,
		Version:  version.Version,
	})

	srv := &http.Server{Addr: listenConfig.BindAddr, Handler: handler}
	listener, err := net.Listen("tcp", listenConfig.BindAddr)
	if err != nil {
		return err
	}

	output.PrintLaunchInfo(version.Version, rootFile, listenConfig.Port.Str)
	if cmd.Bool("open") || cmd.Bool("browser") {
		output.OpenBrowserWhenReady(listenConfig.Port.Str, output.ServedURL(listenConfig.Port.Str))
	}

	if !tailscaleConfig.Enabled {
		return srv.Serve(listener)
	}

	return tailscale.ServeWithServer(ctx, srv, listener, listenConfig.ReadyAddr, listenConfig.Port.Str, tailscaleConfig.Port, cmd.Bool("verbose"), tailscale.RealRunner{})
}

func selectListenConfig(cmd *cli.Command, rootFile string, exposeTailscale bool) (listen.Config, error) {
	return listen.Detect(cmd.String("port"), cmd.IsSet("port"), rootFile, exposeTailscale, listen.IsTCPPortAvailableOnHost, listen.PickLocalPort)
}
