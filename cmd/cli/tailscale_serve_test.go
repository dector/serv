package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCLITailscaleLifecycle(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
if [ "$2" = "status" ]; then
  echo '{}' >> "$TEST_LOG"
  echo '{}'
  exit 0
fi
trap 'exit 0' INT TERM
echo "$*" >> "$TEST_LOG"
echo 'https://host.ts.net/'
while :; do sleep 0.1; done
`
	if err := os.WriteFile(filepath.Join(dir, "tailscale"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_LOG", log)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- newApp().Run(ctx, normalizeArgs([]string{"serv", "--port", port, "-T", "443", dir})) }()
	deadline := time.After(5 * time.Second)
	for {
		data, _ := os.ReadFile(log)
		if strings.Contains(string(data), "serve --yes --https 443 http://127.0.0.1:"+port) {
			if !strings.HasPrefix(string(data), "{}\n{}\n") {
				t.Errorf("preflight and pre-start recheck did not precede startup: %q", data)
			}
			break
		}
		select {
		case err := <-done:
			t.Fatalf("CLI exited early: %v, calls %q", err, data)
		case <-deadline:
			t.Fatalf("CLI did not start: %q", data)
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CLI did not shut down")
	}
}
