package main

import (
	"context"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"

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
	port := choosePort(cmd.String("port"))

	rootFile, err := filepath.Abs(rootFile)
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
			// For directories, we need to get the full path for the FsNode
			fullPath := filepath.Join(rootFile, requestedPath)
			if !rootInfo.IsDir() {
				fullPath = rootFile // serving single file's parent, but this shouldn't happen
			}

			node, err := servfs.GetFsNode(fullPath)
			if err != nil {
				lw.statusCode = 500
				http.Error(lw, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Write(pages.GenerateFolderPage(node, requestedPath, G.Version))
		} else {
			// Serve the file using http.FileServer with the filesystem
			contentType := mime.TypeByExtension(filepath.Ext(requestedPath))
			if contentType != "" {
				lw.Header().Set("Content-Type", contentType)
			}

			fileServer := http.FileServer(http.FS(fsys))
			// Create a new request with the cleaned path
			r.URL.Path = "/" + requestedPath
			fileServer.ServeHTTP(lw, r)
		}
	})

	fmt.Printf("Serving `%s` on http://localhost:%s\n", rootFile, port)
	return http.ListenAndServe(":"+port, nil)
}

func detectContentType(node *servfs.FsNode) (string, error) {
	if node.Info.IsDir() {
		return "", errors.New("not implemented")
	}

	return mime.TypeByExtension(filepath.Ext(node.Path)), nil
}

func choosePort(port string) string {
	portNum, err := strconv.Atoi(port)
	if err != nil {
		fmt.Printf("Warning: invalid port '%s', using default port %d\n", port, DefaultPort)
		portNum = DefaultPort
	}

	if isPortAvailable(portNum) {
		return fmt.Sprintf("%d", portNum)
	}
	fmt.Printf("Warning: port %d is busy, finding available port...\n", portNum)

	// Try up to 100 random ports in the range 10000-20000
	tried := make(map[int]struct{})

	const minPort, maxPort = 10000, 20000
	const maxAttempts = 100
	for attemptsLeft := 100; attemptsLeft > 0; attemptsLeft-- {
		randomPort := func() int {
			return minPort + rand.IntN(maxPort-minPort+1)
		}
		p := randomPort()

		if !isPortAvailable(p) {
			tried[p] = struct{}{}
			continue
		}

		return fmt.Sprintf("%d", p)
	}

	panic("no available ports found in range 10000-20000")
}

func isPortAvailable(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
