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
	"strings"

	"github.com/dector/nettw"
	servfs "github.com/dector/serv/fs"
	"github.com/dector/serv/middleware"
	"github.com/dector/serv/pages"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v3"
)

const servBanner = `
███████╗███████╗██████╗ ██╗   ██╗
██╔════╝██╔════╝██╔══██╗██║   ██║
███████╗█████╗  ██████╔╝██║   ██║
╚════██║██╔══╝  ██╔══██╗╚██╗ ██╔╝
███████║███████╗██║  ██║ ╚████╔╝
╚══════╝╚══════╝╚═╝  ╚═╝  ╚═══╝`

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

func printBanner(version string) {
	lines := strings.Split(servBanner, "\n")
	if len(lines) == 0 {
		return
	}

	if len(lines) > 1 {
		fmt.Println(strings.Join(lines[:len(lines)-1], "\n"))
	}
	fmt.Printf("%s    v. %s\n\n", lines[len(lines)-1], version)
}

func serveAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Bool("version") {
		fmt.Println(G.Version)
		return nil
	}

	rootFile := cmd.StringArg("file")
	rootFile, err := filepath.Abs(rootFile)
	if err != nil {
		return errors.Wrap(err, "failed to get absolute path")
	}

	if _, err := os.Stat(rootFile); os.IsNotExist(err) {
		return errors.Errorf("file does not exist: %s", rootFile)
	}

	port, err := nettw.ParsePortOrPickAnother(cmd.String("port"), nettw.WithSeed(rootFile))
	if err != nil {
		return err
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

	http.HandleFunc("/", middleware.WithLogging(func(w http.ResponseWriter, r *http.Request) {
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
					http.NotFound(w, r)
					return
				}
				requestedPath = basePath
			}
		}

		// Use fs.Stat to get file info securely within the filesystem
		fileInfo, err := fs.Stat(fsys, requestedPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if fileInfo.IsDir() {
			resolveIndex := true
			if cmd.Bool("no-index-resolve") {
				resolveIndex = false
			}

			serveFolder(w, requestedPath, fsys, fileInfo, rootFile, resolveIndex)
		} else {
			serveFile(w, requestedPath, fsys)
		}
	}))

	printBanner(G.Version)
	fmt.Printf("● serving %s\n", rootFile)
	fmt.Printf("  http://localhost:%s\n", port.Str)
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
