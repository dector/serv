package main

import (
	"context"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/dector/nettw"
	servfs "github.com/dector/serv/fs"
	"github.com/dector/serv/pages"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v3"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	lw.statusCode = code
	lw.ResponseWriter.WriteHeader(code)
}

func logRequest(method string, statusCode int, path string) {
	var color string
	if statusCode >= 200 && statusCode < 400 {
		color = "\033[48;5;22m" // Dark green background
	} else {
		color = "\033[48;5;52m" // Dark red background
	}
	reset := "\033[0m"

	fmt.Printf("%s %s %s %d %s\n", color, method, reset, statusCode, path)
}

func main() {
	G.Init()

	app := &cli.Command{
		Name:  "serv",
		Usage: "Serve static files",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Value:   fmt.Sprintf("%d", DefaultPort),
				Usage:   "HTTP server port",
			},
			&cli.BoolFlag{
				Name:    "version",
				Aliases: []string{"v"},
				Value:   false,
				Usage:   "Print version and exit",
			},
			&cli.BoolFlag{
				Name:  "no-index-resolve",
				Value: false,
				Usage: "Disable automatic index.html resolution for directories",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "file",
				UsageText: "path to file or directory to serve",
			},
		},
		Action: serveAction,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		os.Exit(1)
	}
}

func serveAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Bool("version") {
		fmt.Println(G.Version)
		return nil
	}

	rootFile := cmd.StringArg("file")
	port, err := nettw.ParsePortOrPickAnother(cmd.String("port"))
	if err != nil {
		return err
	}

	rootFile, err = filepath.Abs(rootFile)
	if err != nil {
		return errors.Wrap(err, "failed to get absolute path")
	}

	if _, err := os.Stat(rootFile); os.IsNotExist(err) {
		return errors.Errorf("file does not exist: %s", rootFile)
	}

	// Create filesystem rooted at the specified directory or file's parent
	var fsys fs.FS
	var basePath string

	rootInfo, err := os.Stat(rootFile)
	if err != nil {
		return errors.Wrap(err, "failed to stat root file")
	}

	if rootInfo.IsDir() {
		fsys = os.DirFS(rootFile)
		basePath = "."
	} else {
		// If serving a single file, use its parent directory as the filesystem root
		parentDir := filepath.Dir(rootFile)
		fsys = os.DirFS(parentDir)
		basePath = filepath.Base(rootFile)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		lw := &loggingResponseWriter{ResponseWriter: w, statusCode: 200}
		defer func() {
			logRequest(r.Method, lw.statusCode, r.URL.Path)
		}()
		// Clean the URL path and handle root requests
		requestedPath := path.Clean(r.URL.Path)
		if requestedPath == "/" {
			requestedPath = basePath
		} else {
			// For non-root requests, join with basePath if serving a directory
			if rootInfo.IsDir() {
				requestedPath = path.Join(basePath, requestedPath[1:]) // remove leading slash
			} else {
				// If serving a single file, only allow requests to that file
				if requestedPath != "/"+filepath.Base(rootFile) && requestedPath != "/" {
					lw.statusCode = 404
					http.NotFound(lw, r)
					return
				}
				requestedPath = basePath
			}
		}

		// Use fs.Stat to get file info securely within the filesystem
		fileInfo, err := fs.Stat(fsys, requestedPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				lw.statusCode = 404
				http.NotFound(lw, r)
				return
			}
			lw.statusCode = 500
			http.Error(lw, err.Error(), http.StatusInternalServerError)
			return
		}

		if fileInfo.IsDir() {
			resolveIndex := true
			if cmd.Bool("no-index-resolve") {
				resolveIndex = false
			}

			serveFolder(lw, requestedPath, fsys, fileInfo, rootFile, resolveIndex)
		} else {
			serveFile(lw, requestedPath, fsys)
		}
	})

	fmt.Printf("Serving `%s` on http://localhost:%s\n", rootFile, port.Str)
	return http.ListenAndServe(":"+port.Str, nil)
}

func serveFolder(lw http.ResponseWriter, requestedPath string, fsys fs.FS, rootInfo fs.FileInfo, rootFile string, resolveIndex bool) {
	if resolveIndex {
		indexPath := path.Join(requestedPath, "index.html")
		hasIndexHtml := func() bool {
			if indexInfo, err := fs.Stat(fsys, indexPath); err == nil && !indexInfo.IsDir() {
				return true
			}
			return false
		}()
		if hasIndexHtml {
			serveFile(lw, indexPath, fsys)
			return
		}
	}

	// For directories without index.html, we need to get the full path for the FsNode
	fullPath := filepath.Join(rootFile, requestedPath)
	if !rootInfo.IsDir() {
		fullPath = rootFile // serving single file's parent, but this shouldn't happen
	}

	node, err := servfs.GetFsNode(fullPath)
	if err != nil {
		// Set status code to 500 if possible
		if lw, ok := lw.(*loggingResponseWriter); ok {
			lw.statusCode = 500
		}
		http.Error(lw, err.Error(), http.StatusInternalServerError)
		return
	}

	lw.Write(pages.GenerateFolderPage(node, requestedPath, G.Version))
}

func serveFile(lw http.ResponseWriter, requestedPath string, fsys fs.FS) {
	// Serve the file using http.FileServer with the filesystem
	contentType := mime.TypeByExtension(filepath.Ext(requestedPath))
	if contentType != "" {
		lw.Header().Set("Content-Type", contentType)
	}

	content, err := fs.ReadFile(fsys, requestedPath)
	if err != nil {
		http.Error(lw, err.Error(), http.StatusInternalServerError)
		return
	}

	lw.Write(content)
}

func detectContentType(node *servfs.FsNode) (string, error) {
	if node.Info.IsDir() {
		return "", errors.New("not implemented")
	}

	return mime.TypeByExtension(filepath.Ext(node.Path)), nil
}
