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

	"github.com/dector/nettw"
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

func TestDetectListenConfigUsesRequestedPortWhenTailscaleBlocksAllInterfaces(t *testing.T) {
	available := func(host, port string) bool {
		return host == "127.0.0.1" && (port == "8080" || port == "3000")
	}
	picker := func(string, string, bool) (nettw.Port, error) {
		t.Fatal("picker should not be called when requested port is available on localhost")
		return nettw.Port{}, nil
	}

	for _, requestedPort := range []string{"8080", "3000"} {
		t.Run(requestedPort, func(t *testing.T) {
			got, err := detectListenConfig(requestedPort, requestedPort == "3000", "/root", false, available, picker)
			if err != nil {
				t.Fatalf("detectListenConfig() error = %v", err)
			}
			if got.Port.Str != requestedPort {
				t.Fatalf("port = %q, want %q", got.Port.Str, requestedPort)
			}
			if want := net.JoinHostPort("127.0.0.1", requestedPort); got.BindAddr != want || got.ReadyAddr != want {
				t.Fatalf("addresses = bind %q ready %q, want %q", got.BindAddr, got.ReadyAddr, want)
			}
		})
	}
}

func TestDetectListenConfigPicksAnotherWhenLocalhostPortUnavailable(t *testing.T) {
	available := func(host, port string) bool {
		return port == "12345"
	}
	pickerCalled := false
	picker := func(requestedPort, rootFile string, exposeTailscale bool) (nettw.Port, error) {
		pickerCalled = true
		if requestedPort != "8080" || rootFile != "/root" || exposeTailscale {
			t.Fatalf("picker args = (%q, %q, %v), want (8080, /root, false)", requestedPort, rootFile, exposeTailscale)
		}
		return nettw.Port{Str: "12345"}, nil
	}

	got, err := detectListenConfig("8080", false, "/root", false, available, picker)
	if err != nil {
		t.Fatalf("detectListenConfig() error = %v", err)
	}
	if !pickerCalled {
		t.Fatal("expected picker to be called")
	}
	if got.Port.Str != "12345" || got.BindAddr != ":12345" {
		t.Fatalf("detectListenConfig() = %#v, want port 12345 bound on all interfaces", got)
	}
}

func TestDetectListenConfigTailscaleExplicitPortRequiresLocalhostAvailability(t *testing.T) {
	available := func(string, string) bool { return false }
	picker := func(string, string, bool) (nettw.Port, error) {
		t.Fatal("picker should not be called for explicit Tailscale local port")
		return nettw.Port{}, nil
	}

	if _, err := detectListenConfig("3000", true, "/root", true, available, picker); err == nil {
		t.Fatal("expected unavailable explicit Tailscale local port to fail")
	}
}

func TestDetectListenConfigTailscaleWithoutPortPicksRandomHighPort(t *testing.T) {
	available := func(host, port string) bool { return host == "127.0.0.1" && port == "55555" }
	picker := func(requestedPort, rootFile string, exposeTailscale bool) (nettw.Port, error) {
		if requestedPort != "random" || rootFile != "/root" || !exposeTailscale {
			t.Fatalf("picker args = (%q, %q, %v), want (random, /root, true)", requestedPort, rootFile, exposeTailscale)
		}
		return nettw.Port{Str: "55555"}, nil
	}

	got, err := detectListenConfig("8080", false, "/root", true, available, picker)
	if err != nil {
		t.Fatalf("detectListenConfig() error = %v", err)
	}
	if got.Port.Str != "55555" || got.BindAddr != net.JoinHostPort("127.0.0.1", "55555") {
		t.Fatalf("detectListenConfig() = %#v, want Tailscale localhost bind on picked port", got)
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

func TestServeFolderReadmePreviewIncludesSideMenu(t *testing.T) {
	root := t.TempDir()
	rootInfo, err := fstest.MapFS{"docs": &fstest.MapFile{Mode: 0755 | fs.ModeDir}}.Stat("docs")
	if err != nil {
		t.Fatalf("stat root: %v", err)
	}
	fsys := fstest.MapFS{
		"docs":           &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		"docs/README.md": &fstest.MapFile{Data: []byte("# Readme")},
		"docs/a.txt":     &fstest.MapFile{Data: []byte("a")},
		"docs/sub":       &fstest.MapFile{Mode: 0755 | fs.ModeDir},
	}
	r := httptest.NewRequest(http.MethodGet, "/docs/?preview=1&resolve=readme-only", nil)
	w := httptest.NewRecorder()

	serveFolder(w, r, "docs", fsys, rootInfo, root, serveConfig{Mode: serveModeFile, DirResolve: dirResolveReadmeFirst})

	bodyBytes, _ := io.ReadAll(w.Result().Body)
	body := string(bodyBytes)
	for _, want := range []string{"<nav class=\"side-menu\"", "/docs/", "../?preview=1&amp;resolve=readme-only", "sub/?preview=1&amp;resolve=readme-only", "a.txt?preview=1&amp;resolve=readme-only", "[dir]", "side-menu__item--current"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "href=\"README.md") {
		t.Fatalf("current README should not be a link: %s", body)
	}
}

func TestServeFileDirectReadmePreviewOmitsSideMenu(t *testing.T) {
	fsys := fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("# Title")}}
	r := httptest.NewRequest(http.MethodGet, "/README.md", nil)
	w := httptest.NewRecorder()

	serveFile(w, r, "README.md", fsys, serveConfig{Mode: serveModePreview})

	body, _ := io.ReadAll(w.Result().Body)
	if strings.Contains(string(body), "<nav class=\"side-menu\"") {
		t.Fatalf("direct README preview should not include side menu: %s", body)
	}
}

func TestServeFolderIndexPreviewOmitsSideMenu(t *testing.T) {
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
	r := httptest.NewRequest(http.MethodGet, "/docs/?resolve=index-only", nil)
	w := httptest.NewRecorder()

	serveFolder(w, r, "docs", fsys, rootInfo, root, serveConfig{Mode: serveModePreview, DirResolve: dirResolveReadmeFirst})

	body, _ := io.ReadAll(w.Result().Body)
	if strings.Contains(string(body), "<nav class=\"side-menu\"") {
		t.Fatalf("index resolution should not include side menu: %s", body)
	}
}

func TestServeFolderRawReadmeOmitsSideMenu(t *testing.T) {
	root := t.TempDir()
	rootInfo, err := fstest.MapFS{"docs": &fstest.MapFile{Mode: 0755 | fs.ModeDir}}.Stat("docs")
	if err != nil {
		t.Fatalf("stat root: %v", err)
	}
	fsys := fstest.MapFS{
		"docs":           &fstest.MapFile{Mode: 0755 | fs.ModeDir},
		"docs/README.md": &fstest.MapFile{Data: []byte("# Readme")},
	}
	r := httptest.NewRequest(http.MethodGet, "/docs/?raw=1", nil)
	w := httptest.NewRecorder()

	serveFolder(w, r, "docs", fsys, rootInfo, root, serveConfig{Mode: serveModePreview, DirResolve: dirResolveReadmeFirst})

	body, _ := io.ReadAll(w.Result().Body)
	if got := string(body); got != "# Readme" {
		t.Fatalf("body = %q, want raw README", got)
	}
}
