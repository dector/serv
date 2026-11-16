package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/dector/serv/internal/config"
)

// Hide optional interfaces, including io.Seeker, to exercise the fallback.
type nonSeekFS struct{ fs.FS }
type nonSeekFile struct{ fs.File }

func (f nonSeekFS) Open(name string) (fs.File, error) {
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return nonSeekFile{file}, nil
}

func TestServeRawFile(t *testing.T) {
	const content = "0123456789"
	modTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	dir := t.TempDir()
	name := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(name, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(name, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	mapFS := fstest.MapFS{"plain.txt": &fstest.MapFile{Data: []byte(content), ModTime: modTime}}
	filesystems := map[string]fs.FS{
		"disk":         os.DirFS(dir),
		"memory":       mapFS,
		"non-seekable": nonSeekFS{mapFS},
	}
	tests := []struct {
		name, method, byteRange, modifiedSince string
		status                                 int
		body, contentRange, contentLength      string
	}{
		{name: "full", method: http.MethodGet, status: http.StatusOK, body: content, contentLength: "10"},
		{name: "head", method: http.MethodHead, status: http.StatusOK, contentLength: "10"},
		{name: "range", method: http.MethodGet, byteRange: "bytes=2-5", status: http.StatusPartialContent, body: "2345", contentRange: "bytes 2-5/10", contentLength: "4"},
		{name: "suffix", method: http.MethodGet, byteRange: "bytes=-3", status: http.StatusPartialContent, body: "789", contentRange: "bytes 7-9/10", contentLength: "3"},
		{name: "open-ended", method: http.MethodGet, byteRange: "bytes=7-", status: http.StatusPartialContent, body: "789", contentRange: "bytes 7-9/10", contentLength: "3"},
		{name: "unsatisfiable", method: http.MethodGet, byteRange: "bytes=20-", status: http.StatusRequestedRangeNotSatisfiable, contentRange: "bytes */10"},
		{name: "not-modified", method: http.MethodGet, modifiedSince: modTime.Format(http.TimeFormat), status: http.StatusNotModified},
	}
	for fsName, fsys := range filesystems {
		t.Run(fsName, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					r := httptest.NewRequest(tt.method, "/plain.txt", nil)
					if tt.byteRange != "" {
						r.Header.Set("Range", tt.byteRange)
					}
					if tt.modifiedSince != "" {
						r.Header.Set("If-Modified-Since", tt.modifiedSince)
					}
					w := httptest.NewRecorder()
					if err := serveFile(w, r, "plain.txt", fsys, config.Serve{}); err != nil {
						t.Fatal(err)
					}
					if w.Code != tt.status {
						t.Fatalf("status = %d, want %d", w.Code, tt.status)
					}
					if tt.status != http.StatusRequestedRangeNotSatisfiable && w.Body.String() != tt.body {
						t.Errorf("body = %q, want %q", w.Body.String(), tt.body)
					}
					if got := w.Header().Get("Content-Range"); got != tt.contentRange {
						t.Errorf("Content-Range = %q, want %q", got, tt.contentRange)
					}
					if tt.contentLength != "" {
						if got := w.Header().Get("Content-Length"); got != tt.contentLength {
							t.Errorf("Content-Length = %q, want %q", got, tt.contentLength)
						}
						if got := w.Header().Get("Accept-Ranges"); got != "bytes" {
							t.Errorf("Accept-Ranges = %q, want bytes", got)
						}
						if got := w.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
							t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", got)
						}
						if got := w.Header().Get("Last-Modified"); got != modTime.Format(http.TimeFormat) {
							t.Errorf("Last-Modified = %q, want %q", got, modTime.Format(http.TimeFormat))
						}
					}
				})
			}
		})
	}
}
