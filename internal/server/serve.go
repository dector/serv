// Package server implements the HTTP handler that serves files, folders, and
// rendered previews.
package server

import (
	"fmt"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/dector/serv/internal/config"
	servfs "github.com/dector/serv/internal/fs"
	"github.com/dector/serv/internal/middleware"
	"github.com/dector/serv/internal/pages"
	"github.com/dector/serv/internal/preview"
	"github.com/pkg/errors"
)

// Options configures the serving handler.
type Options struct {
	FS       fs.FS
	BasePath string
	RootFile string
	RootInfo fs.FileInfo
	Config   config.Serve
	Version  string
}

// NewHandler builds the root HTTP handler for the given options.
func NewHandler(opts Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", middleware.WithLogging(func(w http.ResponseWriter, r *http.Request) {
		// Clean the URL path and handle root requests
		requestedPath := path.Clean(r.URL.Path)
		if requestedPath == "/" {
			requestedPath = opts.BasePath
		} else {
			// For non-root requests, join with basePath if serving a directory
			if opts.RootInfo.IsDir() {
				requestedPath = path.Join(opts.BasePath, requestedPath[1:]) // remove leading slash
			} else {
				// If serving a single file, only allow requests to that file
				if requestedPath != "/"+filepath.Base(opts.RootFile) && requestedPath != "/" {
					http.NotFound(w, r)
					return
				}
				requestedPath = opts.BasePath
			}
		}

		// Use fs.Stat to get file info securely within the filesystem
		fileInfo, err := fs.Stat(opts.FS, requestedPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if fileInfo.IsDir() {
			serveFolder(w, r, requestedPath, opts, fileInfo)
		} else {
			if err := serveFile(w, r, requestedPath, opts.FS, opts.Config); err != nil {
				serveFileError(w, r, "Serve error", err, nil)
			}
		}
	}))
	return mux
}

type recoveryLink struct {
	Label string
	Query string
}

func shouldRenderPreview(r *http.Request, requestedPath string, mode config.Mode) bool {
	if !preview.CanPreview(requestedPath) {
		return false
	}
	if r != nil && r.URL != nil {
		query := r.URL.Query()
		if query.Get("raw") == "1" {
			return false
		}
		if query.Get("preview") == "1" {
			return true
		}
	}
	return mode == config.ModePreview
}

func urlWithQuery(r *http.Request, query string) string {
	if r == nil || r.URL == nil {
		return "?" + query
	}
	u := *r.URL
	q := u.Query()
	for _, part := range strings.Split(query, "&") {
		key, value, ok := strings.Cut(part, "=")
		if ok {
			q.Set(key, value)
		} else {
			q.Set(key, "")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func directoryRecoveryLinks(strategy config.DirResolve) []recoveryLink {
	links := []recoveryLink{{Label: "Show directory listing", Query: "resolve=none"}}
	switch strategy {
	case config.DirResolveReadmeFirst, config.DirResolveReadmeOnly:
		links = append(links, recoveryLink{Label: "Try index.html", Query: "resolve=index-only"})
	case config.DirResolveIndexFirst, config.DirResolveIndexOnly:
		links = append(links, recoveryLink{Label: "Try README", Query: "resolve=readme-only"})
	}
	return links
}

func serveFolder(lw http.ResponseWriter, r *http.Request, requestedPath string, opts Options, rootInfo fs.FileInfo) {
	strategy := requestDirResolveStrategy(r, opts.Config.DirResolve)
	if strategy != config.DirResolveNone {
		if candidate, ok := selectDirectoryCandidate(opts.FS, requestedPath, strategy); ok {
			if isReadmeName(path.Base(candidate)) && shouldRenderPreview(r, candidate, opts.Config.Mode) {
				if err := servePreviewFile(lw, r, candidate, opts.FS, preview.Options{SideMenu: buildSideMenu(r, requestedPath, candidate, opts.FS)}); err != nil {
					serveFileError(lw, r, "Serve error", err, directoryRecoveryLinks(strategy))
				}
				return
			}
			if err := serveFile(lw, r, candidate, opts.FS, opts.Config); err != nil {
				serveFileError(lw, r, "Serve error", err, directoryRecoveryLinks(strategy))
			}
			return
		}
	}

	fullPath := filepath.Join(opts.RootFile, requestedPath)
	if !rootInfo.IsDir() {
		fullPath = opts.RootFile // serving single file's parent, but this shouldn't happen
	}

	node, err := servfs.GetFsNode(fullPath)
	if err != nil {
		http.Error(lw, err.Error(), http.StatusInternalServerError)
		return
	}

	lw.Write(pages.GenerateFolderPage(node, requestedPath, opts.Version))
}

func serveFile(lw http.ResponseWriter, r *http.Request, requestedPath string, fsys fs.FS, cfg config.Serve) error {
	content, err := fs.ReadFile(fsys, requestedPath)
	if err != nil {
		return err
	}

	if shouldRenderPreview(r, requestedPath, cfg.Mode) {
		return writePreview(lw, r, requestedPath, content, preview.Options{})
	}

	// Serve the file using MIME-by-extension behavior for raw responses.
	contentType := mime.TypeByExtension(filepath.Ext(requestedPath))
	if contentType != "" {
		lw.Header().Set("Content-Type", contentType)
	}
	_, err = lw.Write(content)
	return err
}

func servePreviewFile(lw http.ResponseWriter, r *http.Request, requestedPath string, fsys fs.FS, options preview.Options) error {
	content, err := fs.ReadFile(fsys, requestedPath)
	if err != nil {
		return err
	}
	return writePreview(lw, r, requestedPath, content, options)
}

func writePreview(lw http.ResponseWriter, r *http.Request, requestedPath string, content []byte, options preview.Options) error {
	rendered, err := preview.RenderWithOptions(requestedPath, content, options)
	if err != nil {
		serveFileError(lw, r, "Preview error", err, []recoveryLink{{Label: "View raw file", Query: "raw=1"}})
		return nil
	}
	lw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err = lw.Write(rendered)
	return err
}

func serveFileError(lw http.ResponseWriter, r *http.Request, title string, err error, links []recoveryLink) {
	lw.Header().Set("Content-Type", "text/html; charset=utf-8")
	lw.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(lw, "<!doctype html><html><head><meta charset=\"utf-8\"><title>%s</title></head><body><h1>%s</h1><p>%s</p>", html.EscapeString(title), html.EscapeString(title), html.EscapeString(err.Error()))
	for _, link := range links {
		fmt.Fprintf(lw, "<p><a href=\"%s\">%s</a></p>", html.EscapeString(urlWithQuery(r, link.Query)), html.EscapeString(link.Label))
	}
	fmt.Fprint(lw, "</body></html>")
}
