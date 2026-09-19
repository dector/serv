package server

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dector/serv/internal/config"
)

func TestServeFileDefaultRawMarkdown(t *testing.T) {
	fsys := fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("# Title")}}
	r := httptest.NewRequest(http.MethodGet, "/README.md", nil)
	w := httptest.NewRecorder()

	serveFile(w, r, "README.md", fsys, config.Serve{Mode: config.ModeFile})

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

	serveFile(w, r, "README.md", fsys, config.Serve{Mode: config.ModePreview})

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

	serveFile(w, r, "plain.txt", fsys, config.Serve{Mode: config.ModePreview})

	body, _ := io.ReadAll(w.Result().Body)
	if got := string(body); got != "hello" {
		t.Fatalf("body = %q, want raw text", got)
	}
}

func TestServeFileRawQueryWinsOverPreview(t *testing.T) {
	fsys := fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("# Title")}}
	r := httptest.NewRequest(http.MethodGet, "/README.md?raw=1&preview=1", nil)
	w := httptest.NewRecorder()

	serveFile(w, r, "README.md", fsys, config.Serve{Mode: config.ModePreview})

	body, _ := io.ReadAll(w.Result().Body)
	if got := string(body); got != "# Title" {
		t.Fatalf("body = %q, want raw markdown", got)
	}
}

func TestServeFilePreviewQueryRendersInFileMode(t *testing.T) {
	fsys := fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("# Title")}}
	r := httptest.NewRequest(http.MethodGet, "/README.md?preview=1", nil)
	w := httptest.NewRecorder()

	serveFile(w, r, "README.md", fsys, config.Serve{Mode: config.ModeFile})

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
		strategy config.DirResolve
		want     string
		ok       bool
	}{
		{config.DirResolveReadmeFirst, "docs/README.MD", true},
		{config.DirResolveIndexFirst, "docs/index.html", true},
		{config.DirResolveReadmeOnly, "docs/README.MD", true},
		{config.DirResolveIndexOnly, "docs/index.html", true},
		{config.DirResolveNone, "", false},
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
	if got, ok := selectDirectoryCandidate(fsys, "docs", config.DirResolveReadmeFirst); ok || got != "" {
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

	opts := folderOptions(fsys, root, config.Serve{Mode: config.ModePreview, DirResolve: config.DirResolveReadmeFirst})
	serveFolder(w, r, "docs", opts, rootInfo)

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

	opts := folderOptions(fsys, root, config.Serve{Mode: config.ModeFile, DirResolve: config.DirResolveReadmeFirst})
	serveFolder(w, r, "docs", opts, rootInfo)

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

	serveFile(w, r, "README.md", fsys, config.Serve{Mode: config.ModePreview})

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

	opts := folderOptions(fsys, root, config.Serve{Mode: config.ModePreview, DirResolve: config.DirResolveReadmeFirst})
	serveFolder(w, r, "docs", opts, rootInfo)

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

	opts := folderOptions(fsys, root, config.Serve{Mode: config.ModePreview, DirResolve: config.DirResolveReadmeFirst})
	serveFolder(w, r, "docs", opts, rootInfo)

	body, _ := io.ReadAll(w.Result().Body)
	if got := string(body); got != "# Readme" {
		t.Fatalf("body = %q, want raw README", got)
	}
}

func folderOptions(fsys fs.FS, root string, cfg config.Serve) Options {
	return Options{
		FS:       fsys,
		BasePath: ".",
		RootFile: root,
		Config:   cfg,
	}
}
