package main

import (
	"context"
	"fmt"
	"html"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dector/nettw"
	servfs "github.com/dector/serv/fs"
	"github.com/dector/serv/middleware"
	"github.com/dector/serv/pages"
	"github.com/dector/serv/preview"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
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
			&cli.BoolFlag{
				Name:    "preview",
				Aliases: []string{"P"},
				Value:   false,
				Usage:   "Render supported files as styled HTML previews",
			},
			&cli.BoolFlag{
				Name:    "open",
				Aliases: []string{"o"},
				Value:   false,
				Usage:   "Open the served URL in the default browser",
			},
			&cli.BoolFlag{
				Name:    "browser",
				Aliases: []string{"B"},
				Value:   false,
				Hidden:  true,
				Usage:   "Deprecated: use --open/-o instead",
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

const (
	ansiReset      = "\x1b[0m"
	ansiBold       = "\x1b[1m"
	ansiGreen      = "\x1b[32m"
	ansiMutedGreen = "\x1b[38;5;65m"
	ansiBrightCyan = "\x1b[96m"
	ansiWhite      = "\x1b[37m"
	ansiUnderline  = "\x1b[4m"
)

func shouldUseColor(stdoutIsTerminal bool, lookupEnv func(string) (string, bool)) bool {
	if _, ok := lookupEnv("NO_COLOR"); ok {
		return false
	}
	if termValue, ok := lookupEnv("TERM"); ok && termValue == "dumb" {
		return false
	}
	if forceColor, ok := lookupEnv("FORCE_COLOR"); ok && forceColor != "" && forceColor != "0" {
		return true
	}
	return stdoutIsTerminal
}

func colorize(value string, codes ...string) string {
	return strings.Join(codes, "") + value + ansiReset
}

func printBanner(version string, useColor bool) {
	lines := strings.Split(servBanner, "\n")
	if len(lines) == 0 {
		return
	}

	if len(lines) > 1 {
		bannerHead := strings.Join(lines[:len(lines)-1], "\n")
		if useColor {
			bannerHead = colorize(bannerHead, ansiMutedGreen)
		}
		fmt.Println(bannerHead)
	}

	bannerTail := lines[len(lines)-1]
	versionText := fmt.Sprintf("v. %s", version)
	if useColor {
		bannerTail = colorize(bannerTail, ansiMutedGreen)
		versionText = colorize(versionText, ansiWhite)
	}
	fmt.Printf("%s    %s\n\n", bannerTail, versionText)
}

func browserCommand(goos, url string) (string, []string, bool) {
	switch goos {
	case "linux":
		return "xdg-open", []string{url}, true
	case "darwin":
		return "open", []string{url}, true
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, true
	default:
		return "", nil, false
	}
}

func openBrowser(url string) error {
	name, args, ok := browserCommand(runtime.GOOS, url)
	if !ok {
		return errors.Errorf("opening browser is not supported on %s", runtime.GOOS)
	}
	return exec.Command(name, args...).Start()
}

func waitForTCP(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func openBrowserWhenReady(port, url string) {
	go func() {
		if err := waitForTCP(net.JoinHostPort("127.0.0.1", port), 2*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: server did not become ready for browser launch: %v\n", err)
			return
		}
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to open browser: %v\n", err)
		}
	}()
}

func servedURL(port string) string {
	return fmt.Sprintf("http://localhost:%s", port)
}

func printLaunchInfo(version, rootFile, port string) {
	useColor := shouldUseColor(term.IsTerminal(int(os.Stdout.Fd())), os.LookupEnv)

	printBanner(version, useColor)

	bullet := "●"
	pathText := rootFile
	urlText := servedURL(port)
	if useColor {
		bullet = colorize(bullet, ansiGreen)
		pathText = colorize(pathText, ansiBold)
		urlText = colorize(urlText, ansiBrightCyan, ansiUnderline)
	}

	fmt.Printf("%s serving %s\n", bullet, pathText)
	fmt.Printf("  %s\n", urlText)
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

			serveFolder(w, r, requestedPath, fsys, fileInfo, rootFile, resolveIndex, cmd.Bool("preview"))
		} else {
			serveFile(w, r, requestedPath, fsys, cmd.Bool("preview"))
		}
	}))

	if cmd.Bool("browser") {
		fmt.Fprintln(os.Stderr, "Warning: --browser/-B is deprecated and will be removed in a future release. Use --open/-o instead.")
	}

	printLaunchInfo(G.Version, rootFile, port.Str)
	if cmd.Bool("open") || cmd.Bool("browser") {
		openBrowserWhenReady(port.Str, servedURL(port.Str))
	}
	return http.ListenAndServe(":"+port.Str, nil)
}

func serveFolder(lw http.ResponseWriter, r *http.Request, requestedPath string, fsys fs.FS, rootInfo fs.FileInfo, rootFile string, resolveIndex bool, previewMode bool) {
	if resolveIndex {
		indexPath := path.Join(requestedPath, "index.html")
		hasIndexHtml := func() bool {
			if indexInfo, err := fs.Stat(fsys, indexPath); err == nil && !indexInfo.IsDir() {
				return true
			}
			return false
		}()
		if hasIndexHtml {
			serveFile(lw, r, indexPath, fsys, previewMode)
			return
		}
	}

	// Future: directory README preview fallback could be added later.
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

func serveFile(lw http.ResponseWriter, r *http.Request, requestedPath string, fsys fs.FS, previewMode bool) {
	content, err := fs.ReadFile(fsys, requestedPath)
	if err != nil {
		http.Error(lw, err.Error(), http.StatusInternalServerError)
		return
	}

	if previewMode && preview.CanPreview(requestedPath) {
		// Future: raw access via ?raw / ?raw=1 could be added later.
		rendered, err := preview.Render(requestedPath, content)
		if err != nil {
			servePreviewError(lw, r, err)
			return
		}
		lw.Header().Set("Content-Type", "text/html; charset=utf-8")
		lw.Write(rendered)
		return
	}

	// Serve the file using MIME-by-extension behavior for raw responses.
	contentType := mime.TypeByExtension(filepath.Ext(requestedPath))
	if contentType != "" {
		lw.Header().Set("Content-Type", contentType)
	}
	lw.Write(content)
}

func servePreviewError(lw http.ResponseWriter, r *http.Request, renderErr error) {
	rawURL := "?raw"
	if r != nil && r.URL != nil {
		currentURL := r.URL.String()
		separator := "?"
		if r.URL.RawQuery != "" {
			separator = "&"
		}
		rawURL = currentURL + separator + "raw"
	}
	lw.Header().Set("Content-Type", "text/html; charset=utf-8")
	lw.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(lw, "<!doctype html><html><head><meta charset=\"utf-8\"><title>Preview error</title></head><body><h1>Preview error</h1><p>%s</p><p><a href=\"%s\">View raw file</a></p></body></html>", html.EscapeString(renderErr.Error()), html.EscapeString(rawURL))
}

func detectContentType(node *servfs.FsNode) (string, error) {
	if node.Info.IsDir() {
		return "", errors.New("not implemented")
	}

	return mime.TypeByExtension(filepath.Ext(node.Path)), nil
}
