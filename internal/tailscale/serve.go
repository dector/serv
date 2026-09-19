package tailscale

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	servnet "github.com/dector/serv/internal/netutil"
	"github.com/dector/serv/internal/output"
	"github.com/pkg/errors"
)

// CheckReady verifies that tailscale is installed and the HTTPS port is free.
func CheckReady(ctx context.Context, runner Runner, tailscalePort string) error {
	if err := runner.LookPath(); err != nil {
		return errors.New("tailscale is not installed or not found in PATH; install Tailscale to use --expose-tailscale")
	}

	status, err := runner.StatusJSON(ctx)
	if err != nil {
		if IsNoServeConfig(status) {
			return nil
		}
		return errors.Errorf("failed to check Tailscale Serve status: %v\n%s", err, strings.TrimSpace(string(status)))
	}
	if StatusHasHTTPSPort(status, tailscalePort) {
		return errors.Errorf("Tailscale HTTPS port %s is already configured", tailscalePort)
	}
	return nil
}

// ServeWithServer runs the HTTP server and a tailscale serve process together,
// shutting both down when either exits or a signal arrives.
func ServeWithServer(ctx context.Context, server *http.Server, listener net.Listener, readyAddr, localPort, tailscalePort string, verbose bool, runner Runner) error {
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Serve(listener)
	}()

	if err := servnet.WaitForTCP(readyAddr, 2*time.Second); err != nil {
		_ = server.Shutdown(context.Background())
		return errors.Wrap(err, "server did not become ready for Tailscale exposure")
	}

	stdout := io.Writer(os.Stdout)
	stderr := io.Writer(os.Stderr)
	var urlCh <-chan string
	if !verbose {
		urlWriter := NewURLWriter()
		stdout = urlWriter
		stderr = urlWriter
		urlCh = urlWriter.URL()
	} else {
		fmt.Printf("Exposing via Tailscale HTTPS port %s -> http://127.0.0.1:%s\n", tailscalePort, localPort)
	}
	tailscaleProc, err := runner.StartServe(ctx, tailscalePort, localPort, stdout, stderr)
	if err != nil {
		_ = server.Shutdown(context.Background())
		return errors.Wrap(err, "failed to start tailscale serve")
	}

	tailscaleErr := make(chan error, 1)
	go func() {
		tailscaleErr <- tailscaleProc.Wait()
	}()

	if !verbose {
		select {
		case url := <-urlCh:
			output.PrintURL(url)
		case err := <-tailscaleErr:
			_ = server.Shutdown(context.Background())
			if err != nil {
				return errors.Wrap(err, "tailscale serve failed")
			}
			return nil
		case <-time.After(2 * time.Second):
		}
	}

	signalCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	shutdownTailscale := func() {
		_ = tailscaleProc.Interrupt()
		select {
		case <-tailscaleErr:
		case <-time.After(3 * time.Second):
			_ = tailscaleProc.Kill()
			<-tailscaleErr
		}
	}

	select {
	case err := <-serverErr:
		shutdownTailscale()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-tailscaleErr:
		_ = server.Shutdown(context.Background())
		if err != nil {
			return errors.Wrap(err, "tailscale serve failed")
		}
		return nil
	case <-signalCtx.Done():
		_ = server.Shutdown(context.Background())
		shutdownTailscale()
		return nil
	}
}
