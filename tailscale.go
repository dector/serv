package main

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
)

const exposeTailscaleDefaultValue = "__serv_expose_tailscale_default__"

type tailscaleConfig struct {
	Enabled bool
	Port    string
}

type tailscaleProcess interface {
	Wait() error
	Interrupt() error
	Kill() error
}

type tailscaleRunner interface {
	LookPath() error
	StatusJSON(context.Context) ([]byte, error)
	StartServe(context.Context, string, string, io.Writer, io.Writer) (tailscaleProcess, error)
}

type realTailscaleRunner struct{}

var tailscaleHTTPSURLPattern = regexp.MustCompile(`https://[^\s|]+`)

type tailscaleURLWriter struct {
	mu     sync.Mutex
	text   strings.Builder
	urlCh  chan string
	closed bool
}

func newTailscaleURLWriter() *tailscaleURLWriter {
	return &tailscaleURLWriter{urlCh: make(chan string, 1)}
}

func (w *tailscaleURLWriter) URL() <-chan string {
	return w.urlCh
}

func (w *tailscaleURLWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.text.Write(p)
	if !w.closed {
		if url := parseTailscaleHTTPSURL(w.text.String()); url != "" {
			w.urlCh <- url
			close(w.urlCh)
			w.closed = true
		}
	}
	return len(p), nil
}

func parseTailscaleHTTPSURL(output string) string {
	url := tailscaleHTTPSURLPattern.FindString(output)
	return strings.TrimRight(url, "/.,;:)")
}

func (realTailscaleRunner) LookPath() error {
	_, err := exec.LookPath("tailscale")
	return err
}

func (realTailscaleRunner) StatusJSON(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "tailscale", "serve", "status", "--json")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.Bytes(), err
}

func tailscaleServeArgs(tailscalePort, localPort string) []string {
	return []string{"serve", "--yes", "--https", tailscalePort, "http://127.0.0.1:" + localPort}
}

func (realTailscaleRunner) StartServe(ctx context.Context, tailscalePort, localPort string, stdout, stderr io.Writer) (tailscaleProcess, error) {
	cmd := exec.CommandContext(ctx, "tailscale", tailscaleServeArgs(tailscalePort, localPort)...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execTailscaleProcess{cmd: cmd}, nil
}

type execTailscaleProcess struct {
	cmd *exec.Cmd
}

func (p *execTailscaleProcess) Wait() error { return p.cmd.Wait() }

func (p *execTailscaleProcess) Interrupt() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Signal(os.Interrupt)
}

func (p *execTailscaleProcess) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func parseExposeTailscaleConfig(isSet bool, value string, defaultPort string) (tailscaleConfig, error) {
	if !isSet {
		return tailscaleConfig{}, nil
	}
	if value == "" || value == exposeTailscaleDefaultValue {
		value = defaultPort
	}
	port, err := validateTCPPort(value)
	if err != nil {
		return tailscaleConfig{}, fmt.Errorf("invalid --expose-tailscale port: %w", err)
	}
	return tailscaleConfig{Enabled: true, Port: port}, nil
}

func validateTCPPort(value string) (string, error) {
	port, err := strconv.Atoi(value)
	if err != nil {
		return "", err
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port %d out of range 1-65535", port)
	}
	return strconv.Itoa(port), nil
}

func tailscaleStatusHasHTTPSPort(status []byte, port string) bool {
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

func isNoTailscaleServeConfig(output []byte) bool {
	text := strings.ToLower(string(output))
	return strings.Contains(text, "no serve config") || strings.Contains(text, "serve config is empty") || strings.Contains(text, "no serve configuration")
}

func normalizeExposeTailscaleArgs(args []string) []string {
	out := make([]string, 0, len(args)+1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		out = append(out, arg)
		switch {
		case arg == "-T" || arg == "--expose-tailscale":
			if i+1 >= len(args) || !looksLikePort(args[i+1]) {
				out = append(out, exposeTailscaleDefaultValue)
			}
		case strings.HasPrefix(arg, "--expose-tailscale="):
			if strings.TrimPrefix(arg, "--expose-tailscale=") == "" {
				out[len(out)-1] = "--expose-tailscale=" + exposeTailscaleDefaultValue
			}
		}
	}
	return out
}

func looksLikePort(value string) bool {
	_, err := strconv.Atoi(value)
	return err == nil
}
