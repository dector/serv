package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestShouldUseColor(t *testing.T) {
	tests := []struct {
		name             string
		stdoutIsTerminal bool
		env              map[string]string
		want             bool
	}{
		{
			name:             "terminal enables color",
			stdoutIsTerminal: true,
			want:             true,
		},
		{
			name:             "non-terminal disables color",
			stdoutIsTerminal: false,
			want:             false,
		},
		{
			name:             "force color enables color without terminal",
			stdoutIsTerminal: false,
			env:              map[string]string{"FORCE_COLOR": "1"},
			want:             true,
		},
		{
			name:             "force color zero does not enable color",
			stdoutIsTerminal: false,
			env:              map[string]string{"FORCE_COLOR": "0"},
			want:             false,
		},
		{
			name:             "no color wins over terminal",
			stdoutIsTerminal: true,
			env:              map[string]string{"NO_COLOR": ""},
			want:             false,
		},
		{
			name:             "no color wins over force color",
			stdoutIsTerminal: false,
			env:              map[string]string{"NO_COLOR": "", "FORCE_COLOR": "1"},
			want:             false,
		},
		{
			name:             "dumb terminal disables color",
			stdoutIsTerminal: true,
			env:              map[string]string{"TERM": "dumb"},
			want:             false,
		},
		{
			name:             "dumb terminal wins over force color",
			stdoutIsTerminal: false,
			env:              map[string]string{"TERM": "dumb", "FORCE_COLOR": "1"},
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUseColor(tt.stdoutIsTerminal, func(key string) (string, bool) {
				value, ok := tt.env[key]
				return value, ok
			})
			if got != tt.want {
				t.Fatalf("shouldUseColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBrowserCommand(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		wantName string
		wantArgs []string
		wantOK   bool
	}{
		{name: "linux", goos: "linux", wantName: "xdg-open", wantArgs: []string{"http://localhost:8080"}, wantOK: true},
		{name: "darwin", goos: "darwin", wantName: "open", wantArgs: []string{"http://localhost:8080"}, wantOK: true},
		{name: "windows", goos: "windows", wantName: "rundll32", wantArgs: []string{"url.dll,FileProtocolHandler", "http://localhost:8080"}, wantOK: true},
		{name: "unsupported", goos: "plan9", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotArgs, gotOK := browserCommand(tt.goos, "http://localhost:8080")
			if gotOK != tt.wantOK || gotName != tt.wantName || strings.Join(gotArgs, "\x00") != strings.Join(tt.wantArgs, "\x00") {
				t.Fatalf("browserCommand() = (%q, %q, %v), want (%q, %q, %v)", gotName, gotArgs, gotOK, tt.wantName, tt.wantArgs, tt.wantOK)
			}
		})
	}
}

func TestWaitForTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	if err := waitForTCP(listener.Addr().String(), 200*time.Millisecond); err != nil {
		t.Fatalf("waitForTCP() returned error for listening socket: %v", err)
	}
}

func TestServeFileDefaultRawMarkdown(t *testing.T) {
	fsys := fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("# Title")}}
	r := httptest.NewRequest(http.MethodGet, "/README.md", nil)
	w := httptest.NewRecorder()

	serveFile(w, r, "README.md", fsys, false)

	res := w.Result()
	body, _ := io.ReadAll(res.Body)
	if got := string(body); got != "# Title" {
		t.Fatalf("body = %q, want raw markdown", got)
	}
	if strings.Contains(string(body), "<h1>") {
		t.Fatalf("body rendered unexpectedly: %s", body)
	}
}

func TestServeFilePreviewMarkdown(t *testing.T) {
	fsys := fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("# Title")}}
	r := httptest.NewRequest(http.MethodGet, "/README.md", nil)
	w := httptest.NewRecorder()

	serveFile(w, r, "README.md", fsys, true)

	res := w.Result()
	body, _ := io.ReadAll(res.Body)
	if got := res.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/html; charset=utf-8", got)
	}
	if !strings.Contains(string(body), "<h1>Title</h1>") || !strings.Contains(string(body), "served by serv") {
		t.Fatalf("body did not contain rendered markdown page: %s", body)
	}
}

func TestServeFilePreviewUnsupportedRaw(t *testing.T) {
	fsys := fstest.MapFS{"plain.txt": &fstest.MapFile{Data: []byte("hello")}}
	r := httptest.NewRequest(http.MethodGet, "/plain.txt", nil)
	w := httptest.NewRecorder()

	serveFile(w, r, "plain.txt", fsys, true)

	body, _ := io.ReadAll(w.Result().Body)
	if got := string(body); got != "hello" {
		t.Fatalf("body = %q, want raw text", got)
	}
}
