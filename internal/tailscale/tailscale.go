// Package tailscale integrates the local server with Tailscale Serve.
package tailscale

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	servnet "github.com/dector/serv/internal/netutil"
)

// DefaultExposeValue is a sentinel used when -T/--expose-tailscale is passed
// without a value.
const DefaultExposeValue = "__serv_expose_tailscale_default__"

// Config is the resolved Tailscale exposure configuration.
type Config struct {
	Enabled bool
	Port    string
}

// Process is a running tailscale serve command.
type Process interface {
	Wait() error
	Interrupt() error
	Kill() error
}

// Runner runs tailscale commands.
type Runner interface {
	LookPath() error
	StatusJSON(context.Context) ([]byte, error)
	StartServe(context.Context, string, string, io.Writer, io.Writer) (Process, error)
}

// RealRunner runs the tailscale binary.
type RealRunner struct{}

var httpsURLPattern = regexp.MustCompile(`https://[^\s|]+`)

// URLWriter captures command output and reports the first HTTPS URL it sees.
type URLWriter struct {
	mu     sync.Mutex
	text   strings.Builder
	urlCh  chan string
	closed bool
}

// NewURLWriter creates a URLWriter.
func NewURLWriter() *URLWriter {
	return &URLWriter{urlCh: make(chan string, 1)}
}

// URL returns a channel that receives the first parsed HTTPS URL.
func (w *URLWriter) URL() <-chan string {
	return w.urlCh
}

func (w *URLWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.text.Write(p)
	if !w.closed {
		if url := ParseHTTPSURL(w.text.String()); url != "" {
			w.urlCh <- url
			close(w.urlCh)
			w.closed = true
		}
	}
	return len(p), nil
}

// ParseHTTPSURL extracts the first HTTPS URL from tailscale output.
func ParseHTTPSURL(output string) string {
	url := httpsURLPattern.FindString(output)
	return strings.TrimRight(url, "/.,;:)")
}

// LookPath reports whether the tailscale binary is available.
func (RealRunner) LookPath() error {
	_, err := exec.LookPath("tailscale")
	return err
}

// StatusJSON returns the raw output of tailscale serve status --json.
func (RealRunner) StatusJSON(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "tailscale", "serve", "status", "--json")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.Bytes(), err
}

// ServeArgs builds the tailscale serve arguments.
func ServeArgs(tailscalePort, localPort string) []string {
	return []string{"serve", "--yes", "--https", tailscalePort, "http://127.0.0.1:" + localPort}
}

// StartServe starts tailscale serve for the given ports.
func (RealRunner) StartServe(ctx context.Context, tailscalePort, localPort string, stdout, stderr io.Writer) (Process, error) {
	cmd := exec.CommandContext(ctx, "tailscale", ServeArgs(tailscalePort, localPort)...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcess{cmd: cmd}, nil
}

type execProcess struct {
	cmd *exec.Cmd
}

func (p *execProcess) Wait() error { return p.cmd.Wait() }

func (p *execProcess) Interrupt() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Signal(os.Interrupt)
}

func (p *execProcess) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

// ParseExposeConfig resolves the --expose-tailscale flag into a Config.
func ParseExposeConfig(isSet bool, value string, defaultPort string) (Config, error) {
	if !isSet {
		return Config{}, nil
	}
	if value == "" || value == DefaultExposeValue {
		value = defaultPort
	}
	port, err := servnet.ValidateTCPPort(value)
	if err != nil {
		return Config{}, fmt.Errorf("invalid --expose-tailscale port: %w", err)
	}
	return Config{Enabled: true, Port: port}, nil
}

// StatusHasHTTPSPort reports whether tailscale status already configures port.
func StatusHasHTTPSPort(status []byte, port string) bool {
	var value any
	if err := json.Unmarshal(status, &value); err != nil {
		return false
	}
	return jsonValueHasPort(value, port)
}

func jsonValueHasPort(value any, port string) bool {
	suffix := ":" + port
	slashSuffix := suffix + "/"
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lowerKey := strings.ToLower(key)
			if (strings.Contains(lowerKey, "https") || strings.Contains(lowerKey, ".ts.net") || strings.Contains(lowerKey, ":")) && (key == port || strings.HasSuffix(key, suffix) || strings.Contains(key, slashSuffix)) {
				return true
			}
			if jsonValueHasPort(child, port) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if jsonValueHasPort(child, port) {
				return true
			}
		}
	}
	return false
}

// IsNoServeConfig reports whether status output means no serve config exists.
func IsNoServeConfig(output []byte) bool {
	text := strings.ToLower(string(output))
	return strings.Contains(text, "no serve config") || strings.Contains(text, "serve config is empty") || strings.Contains(text, "no serve configuration")
}

// NormalizeArgs injects the default sentinel when -T is passed without a value
// so the CLI parser keeps the following positional argument.
func NormalizeArgs(args []string) []string {
	out := make([]string, 0, len(args)+1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		out = append(out, arg)
		switch {
		case arg == "-T" || arg == "--expose-tailscale":
			if i+1 >= len(args) || !looksLikePort(args[i+1]) {
				out = append(out, DefaultExposeValue)
			}
		case strings.HasPrefix(arg, "--expose-tailscale="):
			if strings.TrimPrefix(arg, "--expose-tailscale=") == "" {
				out[len(out)-1] = "--expose-tailscale=" + DefaultExposeValue
			}
		}
	}
	return out
}

func looksLikePort(value string) bool {
	_, err := strconv.Atoi(value)
	return err == nil
}
