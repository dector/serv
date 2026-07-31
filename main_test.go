package main

import (
	"io"
	"io/fs"
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

	serveFile(w, r, "README.md", fsys, serveConfig{Mode: serveModeFile})

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

	serveFile(w, r, "README.md", fsys, serveConfig{Mode: serveModePreview})

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

	serveFile(w, r, "plain.txt", fsys, serveConfig{Mode: serveModePreview})

	body, _ := io.ReadAll(w.Result().Body)
	if got := string(body); got != "hello" {
		t.Fatalf("body = %q, want raw text", got)
	}
}

func TestServeFileRawQueryWinsOverPreview(t *testing.T) {
	fsys := fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("# Title")}}
	r := httptest.NewRequest(http.MethodGet, "/README.md?raw=1&preview=1", nil)
	w := httptest.NewRecorder()

	serveFile(w, r, "README.md", fsys, serveConfig{Mode: serveModePreview})

	body, _ := io.ReadAll(w.Result().Body)
	if got := string(body); got != "# Title" {
		t.Fatalf("body = %q, want raw markdown", got)
	}
}

func TestServeFilePreviewQueryRendersInFileMode(t *testing.T) {
	fsys := fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("# Title")}}
	r := httptest.NewRequest(http.MethodGet, "/README.md?preview=1", nil)
	w := httptest.NewRecorder()

	serveFile(w, r, "README.md", fsys, serveConfig{Mode: serveModeFile})

	body, _ := io.ReadAll(w.Result().Body)
	if !strings.Contains(string(body), "<h1>Title</h1>") {
		t.Fatalf("body did not contain rendered markdown page: %s", body)
	}
}

func TestSelectDirectoryCandidateStrategies(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/README.MD":  &fstest.MapFile{Data: []byte("# Readme")},
		"docs/index.html": &fstest.MapFile{Data: []byte("<h1>Index</h1>")},
	}

	tests := []struct {
		strategy dirResolveStrategy
		want     string
		ok       bool
	}{
		{dirResolveReadmeFirst, "docs/README.MD", true},
		{dirResolveIndexFirst, "docs/index.html", true},
		{dirResolveReadmeOnly, "docs/README.MD", true},
		{dirResolveIndexOnly, "docs/index.html", true},
		{dirResolveNone, "", false},
	}

	for _, tt := range tests {
		got, ok := selectDirectoryCandidate(fsys, "docs", tt.strategy)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("selectDirectoryCandidate(%s) = (%q, %v), want (%q, %v)", tt.strategy, got, ok, tt.want, tt.ok)
		}
	}
}

func TestSelectDirectoryCandidateMissingFallsBack(t *testing.T) {
	fsys := fstest.MapFS{"docs/file.txt": &fstest.MapFile{Data: []byte("x")}}
	if got, ok := selectDirectoryCandidate(fsys, "docs", dirResolveReadmeFirst); ok || got != "" {
		t.Fatalf("selectDirectoryCandidate() = (%q, %v), want no candidate", got, ok)
	}
}

func TestServeFolderPreviewDefaultServesReadmeBeforeIndex(t *testing.T) {
	root := t.TempDir()
	rootInfo, err := fstest.MapFS{"docs": &fstest.MapFile{Mode: 0755 | fs.ModeDir}}.Stat("docs")
	if err != nil {
		t.Fatalf("stat root: %v", err)
	}
	fsys := fstest.MapFS{
		"docs":            &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		"docs/README.md":  &fstest.MapFile{Data: []byte("# Readme")},
		"docs/index.html": &fstest.MapFile{Data: []byte("<h1>Index</h1>")},
	}
	r := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	w := httptest.NewRecorder()

	serveFolder(w, r, "docs", fsys, rootInfo, root, serveConfig{Mode: serveModePreview, DirResolve: dirResolveReadmeFirst})

	body, _ := io.ReadAll(w.Result().Body)
	if !strings.Contains(string(body), "<h1>Readme</h1>") || strings.Contains(string(body), "Index") {
		t.Fatalf("body = %s, want README preview", body)
	}
}
