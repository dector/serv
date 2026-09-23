// Package tailscale manages a foreground Tailscale Serve process, not a local HTTP server.
package tailscale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Action controls preflight and process startup. Default is CheckAndStart.
type Action uint8

const (
	Default Action = iota
	OnlyCheck
	CheckAndStart
	OnlyStart
)

// Config describes the already-listening local backend and Tailscale HTTPS port.
// Stdout and Stderr are optional; output is discarded when nil, but URL discovery still works.
type Config struct {
	LocalAddr string
	HTTPSPort int
	Action    Action
	// AllowReplaceExisting explicitly permits OnlyStart to skip the occupancy check.
	// It does not make CheckAndStart atomic: a concurrent update may still be replaced.
	AllowReplaceExisting bool
	Stdout, Stderr       io.Writer
}

type process interface {
	Wait() error
	Interrupt() error
	Kill() error
}
type runner interface {
	LookPath() error
	StatusJSON(context.Context) ([]byte, error)
	StartServe(context.Context, string, string, io.Writer, io.Writer) (process, error)
}

var commandRunner runner = realRunner{}

type realRunner struct{}

func (realRunner) LookPath() error { _, err := exec.LookPath("tailscale"); return err }
func (realRunner) StatusJSON(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "tailscale", "serve", "status", "--json")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.Bytes(), err
}
func serveArgs(port string, target string) []string {
	return []string{"serve", "--yes", "--https", port, target}
}
func (realRunner) StartServe(ctx context.Context, port, target string, stdout, stderr io.Writer) (process, error) {
	// Session owns termination so that Wait can always reap the process.
	cmd := exec.Command("tailscale", serveArgs(port, target)...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcess{cmd}, nil
}

type execProcess struct{ cmd *exec.Cmd }

func (p *execProcess) Wait() error      { return p.cmd.Wait() }
func (p *execProcess) Interrupt() error { return p.cmd.Process.Signal(os.Interrupt) }
func (p *execProcess) Kill() error      { return p.cmd.Process.Kill() }

// Session owns a foreground process. Wait is repeatable and safe alongside Close.
type Session struct {
	proc      process
	url       *urlWriter
	done      chan struct{}
	mu        sync.Mutex
	err       error
	closeOnce sync.Once
}

// URL yields at most the first HTTPS URL, then closes; it closes without a value if the process exits first.
func (s *Session) URL() <-chan string { return s.url.ch }

// Wait waits for process exit and returns its result (including errors after forced shutdown).
func (s *Session) Wait() error { <-s.done; s.mu.Lock(); defer s.mu.Unlock(); return s.err }

// Close interrupts the process, kills it after three seconds if needed, and always waits for reaping.
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		select {
		case <-s.done:
			return
		default:
		}
		_ = s.proc.Interrupt()
		select {
		case <-s.done:
		case <-time.After(3 * time.Second):
			_ = s.proc.Kill()
			<-s.done
		}
	})
	return nil
}

// Start validates config, optionally checks the port, and starts a foreground Serve process.
// OnlyCheck returns (nil, nil) on success. OnlyStart skips the non-atomic status
// preflight and requires AllowReplaceExisting, since --yes may overwrite a config.
func Start(ctx context.Context, cfg Config) (*Session, error) { return start(ctx, cfg, commandRunner) }
func start(ctx context.Context, cfg Config, r runner) (*Session, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.HTTPSPort < 1 || cfg.HTTPSPort > 65535 {
		return nil, fmt.Errorf("invalid HTTPS port %d: must be 1-65535", cfg.HTTPSPort)
	}
	action := cfg.Action
	if action == Default {
		action = CheckAndStart
	}
	if action != OnlyCheck && action != CheckAndStart && action != OnlyStart {
		return nil, fmt.Errorf("invalid tailscale action %d", action)
	}
	if action == OnlyStart && !cfg.AllowReplaceExisting {
		return nil, errors.New("OnlyStart requires AllowReplaceExisting: tailscale serve --yes may replace an existing configuration")
	}
	var target string
	if cfg.LocalAddr != "" || action != OnlyCheck {
		host, port, err := net.SplitHostPort(cfg.LocalAddr)
		if err != nil {
			return nil, fmt.Errorf("invalid local address: %w", err)
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid local port %q: must be 1-65535", port)
		}
		if host == "" {
			host = "127.0.0.1"
		}
		if strings.ContainsAny(host, "/?#@") {
			return nil, fmt.Errorf("invalid local host %q", host)
		}
		target = (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(n))}).String()
	}
	portStr := strconv.Itoa(cfg.HTTPSPort)
	if action != OnlyStart {
		if err := checkReady(ctx, r, portStr); err != nil {
			return nil, err
		}
	}
	if action == OnlyCheck {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	capture := newURLWriter()
	stdout, stderr := io.Writer(capture.stream(0)), io.Writer(capture.stream(1))
	if cfg.Stdout != nil {
		stdout = io.MultiWriter(stdout, cfg.Stdout)
	}
	if cfg.Stderr != nil {
		stderr = io.MultiWriter(stderr, cfg.Stderr)
	}
	proc, err := r.StartServe(ctx, portStr, target, stdout, stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to start tailscale serve: %w", err)
	}
	s := &Session{proc: proc, url: capture, done: make(chan struct{})}
	go func() {
		err := proc.Wait()
		capture.close()
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		close(s.done)
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-s.done:
		}
	}()
	return s, nil
}

func checkReady(ctx context.Context, r runner, port string) error {
	if err := r.LookPath(); err != nil {
		return errors.New("tailscale is not installed or not found in PATH; install Tailscale to use --expose-tailscale")
	}
	status, err := r.StatusJSON(ctx)
	if err != nil {
		if isNoServeConfig(status) {
			return nil
		}
		return fmt.Errorf("failed to check Tailscale Serve status: %v\n%s", err, strings.TrimSpace(string(status)))
	}
	if !json.Valid(status) {
		return fmt.Errorf("invalid Tailscale Serve status JSON: %q", strings.TrimSpace(string(status)))
	}
	if statusHasHTTPSPort(status, port) {
		return fmt.Errorf("Tailscale HTTPS port %s is already configured", port)
	}
	return nil
}
func isNoServeConfig(output []byte) bool {
	text := strings.ToLower(string(output))
	return strings.Contains(text, "no serve config") || strings.Contains(text, "serve config is empty") || strings.Contains(text, "no serve configuration")
}
func statusHasHTTPSPort(status []byte, port string) bool {
	var root struct {
		TCP map[string]struct {
			HTTPS bool `json:"HTTPS"`
		} `json:"TCP"`
		Web         map[string]json.RawMessage `json:"Web"`
		ServeConfig json.RawMessage            `json:"ServeConfig"`
	}
	if json.Unmarshal(status, &root) != nil {
		return false
	}
	if tcp, ok := root.TCP[port]; ok && tcp.HTTPS {
		return true
	}
	for host := range root.Web {
		_, p, err := net.SplitHostPort(host)
		if err == nil && p == port {
			return true
		}
	}
	if len(root.ServeConfig) != 0 && string(root.ServeConfig) != "null" {
		return statusHasHTTPSPort(root.ServeConfig, port)
	}
	return false
}

var httpsURLPattern = regexp.MustCompile(`https://[^\s|]+`)

// Each output stream has its own buffer. Only publication of the first URL is shared.
type urlWriter struct {
	mu      sync.Mutex
	streams [2]urlStream
	ch      chan string
	closed  bool
}

type urlStream struct {
	owner      *urlWriter
	text       strings.Builder
	timer      *time.Timer
	generation uint64
}

const urlQuietPeriod = 100 * time.Millisecond

func newURLWriter() *urlWriter {
	w := &urlWriter{ch: make(chan string, 1)}
	for i := range w.streams {
		w.streams[i].owner = w
	}
	return w
}

func (w *urlWriter) stream(i int) io.Writer { return &w.streams[i] }

// Write preserves the original single-stream writer behavior for internal callers.
func (w *urlWriter) Write(p []byte) (int, error) { return w.streams[0].Write(p) }

func (s *urlStream) Write(p []byte) (int, error) {
	w := s.owner
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		s.generation++
		s.text.Write(p)
		text := s.text.String()
		if loc := httpsURLPattern.FindStringIndex(text); loc != nil {
			if loc[1] < len(text) {
				w.publish(text[loc[0]:loc[1]])
			} else {
				// A write boundary is not a URL boundary. Wait briefly for more output.
				if s.timer != nil {
					s.timer.Stop()
				}
				generation := s.generation
				s.timer = time.AfterFunc(urlQuietPeriod, func() {
					w.mu.Lock()
					defer w.mu.Unlock()
					if !w.closed && s.generation == generation {
						w.publish(httpsURLPattern.FindString(s.text.String()))
					}
				})
			}
		}
	}
	return len(p), nil
}
func (w *urlWriter) publish(value string) {
	value = strings.TrimRight(value, "/.,;:)")
	if value == "" || w.closed {
		return
	}
	w.ch <- value
	close(w.ch)
	w.closed = true
	for i := range w.streams {
		if timer := w.streams[i].timer; timer != nil {
			timer.Stop()
		}
	}
}
func (w *urlWriter) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		for i := range w.streams {
			w.publish(httpsURLPattern.FindString(w.streams[i].text.String()))
		}
		if !w.closed {
			close(w.ch)
			w.closed = true
		}
		for i := range w.streams {
			if timer := w.streams[i].timer; timer != nil {
				timer.Stop()
			}
		}
	}
}
