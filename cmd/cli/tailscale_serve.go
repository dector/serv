package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	servnet "github.com/dector/serv/internal/netutil"
	"github.com/dector/serv/internal/output"
	"github.com/dector/serv/internal/tailscale"
	"github.com/pkg/errors"
)

// serveWithTailscale owns the HTTP server, readiness, signals, and CLI output.
func serveWithTailscale(ctx context.Context, srv *http.Server, listener net.Listener, readyAddr, localPort, httpsPort string, verbose bool) error {
	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.Serve(listener) }()
	if err := servnet.WaitForTCP(readyAddr, 2*time.Second); err != nil {
		_ = srv.Shutdown(context.Background())
		return errors.Wrap(err, "server did not become ready for Tailscale exposure")
	}
	// Recheck after the early CLI preflight rather than opting into replacement.
	cfg := tailscale.Config{LocalAddr: net.JoinHostPort("127.0.0.1", localPort), Action: tailscale.CheckAndStart}
	port, err := strconv.Atoi(httpsPort)
	cfg.HTTPSPort = port
	if err != nil {
		_ = srv.Shutdown(context.Background())
		return err
	}
	if verbose {
		fmt.Printf("Exposing via Tailscale HTTPS port %s -> http://127.0.0.1:%s\n", httpsPort, localPort)
		cfg.Stdout, cfg.Stderr = os.Stdout, os.Stderr
	}
	session, err := tailscale.Start(ctx, cfg)
	if err != nil {
		_ = srv.Shutdown(context.Background())
		return err
	}
	defer session.Close()
	tailscaleErr := make(chan error, 1)
	go func() { tailscaleErr <- session.Wait() }()
	if !verbose {
		select {
		case url, ok := <-session.URL():
			if ok {
				output.PrintURL(url)
			}
		case err := <-tailscaleErr:
			_ = srv.Shutdown(context.Background())
			if err != nil {
				return errors.Wrap(err, "tailscale serve failed")
			}
			return nil
		case <-time.After(2 * time.Second):
		}
	}
	signalCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-tailscaleErr:
		_ = srv.Shutdown(context.Background())
		if err != nil {
			return errors.Wrap(err, "tailscale serve failed")
		}
		return nil
	case <-signalCtx.Done():
		_ = srv.Shutdown(context.Background())
		return nil
	}
}
