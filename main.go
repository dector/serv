package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
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
			&cli.StringFlag{
				Name:    "mode",
				Aliases: []string{"m"},
				Value:   string(serveModePreview),
				Usage:   "Serving mode: preview/p or file/f",
			},
			&cli.StringFlag{
				Name:  "dir-resolve",
				Usage: "Directory resolution: readme-first/rf, index-first/if, readme-only/ro, index-only/io, none/n",
			},
			&cli.BoolFlag{
				Name:   "no-index-resolve",
				Value:  false,
				Hidden: true,
				Usage:  "Deprecated: use --dir-resolve instead",
			},
			&cli.BoolFlag{
				Name:    "preview",
				Aliases: []string{"P"},
				Value:   false,
				Hidden:  true,
				Usage:   "Deprecated: use --mode preview instead",
			},
			&cli.BoolFlag{
				Name:    "open",
				Aliases: []string{"o"},
				Value:   false,
				Usage:   "Open the served URL in the default browser",
			},
			&cli.StringFlag{
				Name:        "expose-tailscale",
				Aliases:     []string{"T"},
				Value:       exposeTailscaleDefaultValue,
				DefaultText: "local port",
				HideDefault: true,
				Usage:       "Expose via Tailscale Serve; optional value sets HTTPS port",
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Value: false,
				Usage: "Print verbose command output",
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

	if err := app.Run(context.Background(), normalizeExposeTailscaleArgs(os.Args)); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		os.Exit(1)
	}
}

type serveMode string

type dirResolveStrategy string

type serveConfig struct {
	Mode       serveMode
	DirResolve dirResolveStrategy
}

type recoveryLink struct {
	Label string
	Query string
}

const (
	serveModePreview serveMode = "preview"
	serveModeFile    serveMode = "file"

	dirResolveReadmeFirst dirResolveStrategy = "readme-first"
	dirResolveIndexFirst  dirResolveStrategy = "index-first"
	dirResolveReadmeOnly  dirResolveStrategy = "readme-only"
	dirResolveIndexOnly   dirResolveStrategy = "index-only"
	dirResolveNone        dirResolveStrategy = "none"
)

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
	if useColor {
		bullet = colorize(bullet, ansiGreen)
		pathText = colorize(pathText, ansiBold)
	}

	fmt.Printf("%s serving %s\n", bullet, pathText)
	printURLLine(servedURL(port))
}

func printURLLine(url string) {
	useColor := shouldUseColor(term.IsTerminal(int(os.Stdout.Fd())), os.LookupEnv)
	urlText := url
	if useColor {
		urlText = colorize(urlText, ansiBrightCyan, ansiUnderline)
	}
	fmt.Printf("  %s\n", urlText)
}

func parseServeMode(value string) (serveMode, error) {
	switch strings.ToLower(value) {
	case "preview", "p", "":
		return serveModePreview, nil
	case "file", "f":
		return serveModeFile, nil
	default:
		return "", errors.Errorf("invalid mode %q (expected preview/p or file/f)", value)
	}
}

func parseDirResolveStrategy(value string) (dirResolveStrategy, error) {
	switch strings.ToLower(value) {
	case "readme-first", "rf", "":
		return dirResolveReadmeFirst, nil
	case "index-first", "if":
		return dirResolveIndexFirst, nil
	case "readme-only", "ro":
		return dirResolveReadmeOnly, nil
	case "index-only", "io":
		return dirResolveIndexOnly, nil
	case "none", "n":
		return dirResolveNone, nil
	default:
		return "", errors.Errorf("invalid dir-resolve %q", value)
	}
}

func effectiveServeConfig(cmd *cli.Command) (serveConfig, error) {
	mode, err := parseServeMode(cmd.String("mode"))
	if err != nil {
		return serveConfig{}, err
	}

	if cmd.Bool("preview") {
		fmt.Fprintln(os.Stderr, "Warning: --preview is deprecated; use --mode preview instead.")
		if cmd.IsSet("mode") && mode == serveModeFile {
			return serveConfig{}, errors.New("--preview cannot be combined with --mode file")
		}
		mode = serveModePreview
	}

	strategy := dirResolveReadmeFirst
	if mode == serveModeFile {
		strategy = dirResolveNone
	}
	if cmd.IsSet("dir-resolve") {
		strategy, err = parseDirResolveStrategy(cmd.String("dir-resolve"))
		if err != nil {
			return serveConfig{}, err
		}
	}
	if cmd.Bool("no-index-resolve") {
		fmt.Fprintln(os.Stderr, "Warning: --no-index-resolve is deprecated; use --dir-resolve instead.")
		if cmd.IsSet("dir-resolve") {
			fmt.Fprintln(os.Stderr, "Warning: --no-index-resolve ignored because --dir-resolve was provided.")
		} else if mode == serveModePreview {
			strategy = dirResolveReadmeOnly
		}
	}

	return serveConfig{Mode: mode, DirResolve: strategy}, nil
}

func listenAddr(host, port string) string {
	if host == "" {
		return ":" + port
	}
	return net.JoinHostPort(host, port)
}

func isTCPPortAvailableOnHost(host, port string) bool {
	ln, err := net.Listen("tcp", listenAddr(host, port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func isTCPPortAvailable(port string) bool {
	return isTCPPortAvailableOnHost("127.0.0.1", port)
}

type listenConfig struct {
	Port      nettw.Port
	BindAddr  string
	ReadyAddr string
}

type portAvailabilityFunc func(host, port string) bool

type portPickerFunc func(requestedPort, rootFile string, exposeTailscale bool) (nettw.Port, error)

func pickLocalPort(requestedPort, rootFile string, exposeTailscale bool) (nettw.Port, error) {
	if exposeTailscale {
		return nettw.ParsePortOrPickAnother("random", nettw.WithIgnoreInvalidPort(true), nettw.WithSeed(rootFile), nettw.WithPortRange(49152, 65535))
	}
	return nettw.ParsePortOrPickAnother(requestedPort, nettw.WithSeed(rootFile))
}

func detectListenConfig(requestedPort string, portIsSet bool, rootFile string, exposeTailscale bool, available portAvailabilityFunc, pickPort portPickerFunc) (listenConfig, error) {
	var port nettw.Port
	if !exposeTailscale {
		if parsedPort, err := validateTCPPort(requestedPort); err == nil && available("127.0.0.1", parsedPort) {
			port = nettw.Port{Str: parsedPort}
		} else {
			pickedPort, err := pickPort(requestedPort, rootFile, false)
			if err != nil {
				return listenConfig{}, err
			}
			port = pickedPort
		}
	} else if portIsSet {
		parsedPort, err := validateTCPPort(requestedPort)
		if err != nil {
			return listenConfig{}, fmt.Errorf("invalid --port: %w", err)
		}
		if !available("127.0.0.1", parsedPort) {
			return listenConfig{}, errors.Errorf("local port %s is not available", parsedPort)
		}
		port = nettw.Port{Str: parsedPort}
	} else {
		pickedPort, err := pickPort("random", rootFile, true)
		if err != nil {
			return listenConfig{}, err
		}
		port = pickedPort
	}

	readyAddr := net.JoinHostPort("127.0.0.1", port.Str)
	bindAddr := listenAddr("", port.Str)
	if exposeTailscale || !available("", port.Str) {
		bindAddr = readyAddr
	}
	return listenConfig{Port: port, BindAddr: bindAddr, ReadyAddr: readyAddr}, nil
}

func selectListenConfig(cmd *cli.Command, rootFile string, exposeTailscale bool) (listenConfig, error) {
	return detectListenConfig(cmd.String("port"), cmd.IsSet("port"), rootFile, exposeTailscale, isTCPPortAvailableOnHost, pickLocalPort)
}

func checkTailscaleReady(ctx context.Context, runner tailscaleRunner, tailscalePort string) error {
	if err := runner.LookPath(); err != nil {
		return errors.New("tailscale is not installed or not found in PATH; install Tailscale to use --expose-tailscale")
	}

	status, err := runner.StatusJSON(ctx)
	if err != nil {
		if isNoTailscaleServeConfig(status) {
			return nil
		}
		return errors.Errorf("failed to check Tailscale Serve status: %v\n%s", err, strings.TrimSpace(string(status)))
	}
	if tailscaleStatusHasHTTPSPort(status, tailscalePort) {
		return errors.Errorf("Tailscale HTTPS port %s is already configured", tailscalePort)
	}
	return nil
}

func serveWithTailscale(ctx context.Context, server *http.Server, listener net.Listener, readyAddr, localPort, tailscalePort string, verbose bool, runner tailscaleRunner) error {
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Serve(listener)
	}()

	if err := waitForTCP(readyAddr, 2*time.Second); err != nil {
		_ = server.Shutdown(context.Background())
		return errors.Wrap(err, "server did not become ready for Tailscale exposure")
	}

	stdout := io.Writer(os.Stdout)
	stderr := io.Writer(os.Stderr)
	var urlCh <-chan string
	if !verbose {
		urlWriter := newTailscaleURLWriter()
		stdout = urlWriter
		stderr = urlWriter
		urlCh = urlWriter.URL()
	} else {
		fmt.Printf("Exposing via Tailscale HTTPS port %s -> http://127.0.0.1:%s\n", tailscalePort, localPort)
	}
	tailscaleProc, err := runner.StartServe(ctx, tailscalePort, localPort, stdout, stderr)
	if err != nil {
		_ = server.Shutdown(context.Background())
		return errors.Wrap(err, "failed to start tailscale serve")
	}

	tailscaleErr := make(chan error, 1)
	go func() {
		tailscaleErr <- tailscaleProc.Wait()
	}()

	if !verbose {
		select {
		case url := <-urlCh:
			printURLLine(url)
		case err := <-tailscaleErr:
			_ = server.Shutdown(context.Background())
			if err != nil {
				return errors.Wrap(err, "tailscale serve failed")
			}
			return nil
		case <-time.After(2 * time.Second):
		}
	}

	signalCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	shutdownTailscale := func() {
		_ = tailscaleProc.Interrupt()
		select {
		case <-tailscaleErr:
		case <-time.After(3 * time.Second):
			_ = tailscaleProc.Kill()
			<-tailscaleErr
		}
	}

	select {
	case err := <-serverErr:
		shutdownTailscale()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-tailscaleErr:
		_ = server.Shutdown(context.Background())
		if err != nil {
			return errors.Wrap(err, "tailscale serve failed")
		}
		return nil
	case <-signalCtx.Done():
		_ = server.Shutdown(context.Background())
		shutdownTailscale()
		return nil
	}
}

func serveAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Bool("version") {
		fmt.Println(G.Version)
		return nil
	}

	serveConfig, err := effectiveServeConfig(cmd)
	if err != nil {
		return err
	}

	rootFile := cmd.StringArg("file")
	rootFile, err = filepath.Abs(rootFile)
	if err != nil {
		return errors.Wrap(err, "failed to get absolute path")
	}

	if _, err := os.Stat(rootFile); os.IsNotExist(err) {
		return errors.Errorf("file does not exist: %s", rootFile)
	}

	exposeTailscale := cmd.IsSet("expose-tailscale")
	listenConfig, err := selectListenConfig(cmd, rootFile, exposeTailscale)
	if err != nil {
		return err
	}
	tailscaleConfig, err := parseExposeTailscaleConfig(exposeTailscale, cmd.String("expose-tailscale"), listenConfig.Port.Str)
	if err != nil {
		return err
	}
	if tailscaleConfig.Enabled {
		if err := checkTailscaleReady(ctx, realTailscaleRunner{}, tailscaleConfig.Port); err != nil {
			return err
		}
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

	mux := http.NewServeMux()
	mux.HandleFunc("/", middleware.WithLogging(func(w http.ResponseWriter, r *http.Request) {
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
			serveFolder(w, r, requestedPath, fsys, fileInfo, rootFile, serveConfig)
		} else {
			if err := serveFile(w, r, requestedPath, fsys, serveConfig); err != nil {
				serveFileError(w, r, "Serve error", err, nil)
			}
		}
	}))

	if cmd.Bool("browser") {
		fmt.Fprintln(os.Stderr, "Warning: --browser/-B is deprecated and will be removed in a future release. Use --open/-o instead.")
	}

	server := &http.Server{Addr: listenConfig.BindAddr, Handler: mux}
	listener, err := net.Listen("tcp", listenConfig.BindAddr)
	if err != nil {
		return err
	}

	printLaunchInfo(G.Version, rootFile, listenConfig.Port.Str)
	if cmd.Bool("open") || cmd.Bool("browser") {
		openBrowserWhenReady(listenConfig.Port.Str, servedURL(listenConfig.Port.Str))
	}

	if !tailscaleConfig.Enabled {
		return server.Serve(listener)
	}

	return serveWithTailscale(ctx, server, listener, listenConfig.ReadyAddr, listenConfig.Port.Str, tailscaleConfig.Port, cmd.Bool("verbose"), realTailscaleRunner{})
}

func requestDirResolveStrategy(r *http.Request, fallback dirResolveStrategy) dirResolveStrategy {
	if r == nil || r.URL == nil {
		return fallback
	}
	value := r.URL.Query().Get("resolve")
	if value == "" {
		return fallback
	}
	strategy, err := parseDirResolveStrategy(value)
	if err != nil {
		return fallback
	}
	return strategy
}

func directoryCandidatePaths(fsys fs.FS, dir string, strategy dirResolveStrategy) []string {
	readme := func() []string {
		if p, ok := findReadme(fsys, dir); ok {
			return []string{p}
		}
		return nil
	}
	index := []string{path.Join(dir, "index.html")}
	switch strategy {
	case dirResolveReadmeFirst:
		return append(readme(), index...)
	case dirResolveIndexFirst:
		return append(index, readme()...)
	case dirResolveReadmeOnly:
		return readme()
	case dirResolveIndexOnly:
		return index
	default:
		return nil
	}
}

func selectDirectoryCandidate(fsys fs.FS, dir string, strategy dirResolveStrategy) (string, bool) {
	for _, candidate := range directoryCandidatePaths(fsys, dir, strategy) {
		info, err := fs.Stat(fsys, candidate)
		if err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func findReadme(fsys fs.FS, dir string) (string, bool) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if isReadmeName(entry.Name()) {
			return path.Join(dir, entry.Name()), true
		}
	}
	return "", false
}

func isReadmeName(name string) bool {
	allowed := map[string]bool{"readme.md": true, "readme.markdown": true, "readme.mdown": true, "readme.mkd": true}
	return allowed[strings.ToLower(name)]
}

func buildSideMenu(r *http.Request, dir string, currentPath string, fsys fs.FS) *preview.SideMenu {
	menu := &preview.SideMenu{CurrentPath: displayDirPath(dir)}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		menu.Error = "Could not load directory menu."
		return menu
	}

	if !isRootDirPath(dir) {
		menu.Items = append(menu.Items, preview.SideMenuItem{
			Label: "../",
			Href:  sideMenuHref(r, "../"),
			Title: "../",
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	currentName := path.Base(currentPath)
	for _, entry := range entries {
		name := entry.Name()
		label := name
		href := sideMenuHref(r, name)
		if entry.IsDir() {
			label = name + "/"
			href = sideMenuHref(r, name+"/")
		}
		menu.Items = append(menu.Items, preview.SideMenuItem{
			Label:     label,
			Href:      href,
			Title:     label,
			IsDir:     entry.IsDir(),
			IsCurrent: !entry.IsDir() && name == currentName,
		})
	}
	return menu
}

func displayDirPath(dir string) string {
	clean := path.Clean(dir)
	if clean == "." || clean == "/" {
		return "/"
	}
	return "/" + strings.Trim(clean, "/") + "/"
}

func isRootDirPath(dir string) bool {
	clean := path.Clean(dir)
	return clean == "." || clean == "/"
}

func sideMenuHref(r *http.Request, href string) string {
	if r == nil || r.URL == nil || r.URL.RawQuery == "" {
		return href
	}
	return href + "?" + r.URL.RawQuery
}

func shouldRenderPreview(r *http.Request, requestedPath string, mode serveMode) bool {
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
	return mode == serveModePreview
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

func directoryRecoveryLinks(strategy dirResolveStrategy) []recoveryLink {
	links := []recoveryLink{{Label: "Show directory listing", Query: "resolve=none"}}
	switch strategy {
	case dirResolveReadmeFirst, dirResolveReadmeOnly:
		links = append(links, recoveryLink{Label: "Try index.html", Query: "resolve=index-only"})
	case dirResolveIndexFirst, dirResolveIndexOnly:
		links = append(links, recoveryLink{Label: "Try README", Query: "resolve=readme-only"})
	}
	return links
}

func serveFolder(lw http.ResponseWriter, r *http.Request, requestedPath string, fsys fs.FS, rootInfo fs.FileInfo, rootFile string, config serveConfig) {
	strategy := requestDirResolveStrategy(r, config.DirResolve)
	if strategy != dirResolveNone {
		if candidate, ok := selectDirectoryCandidate(fsys, requestedPath, strategy); ok {
			if isReadmeName(path.Base(candidate)) && shouldRenderPreview(r, candidate, config.Mode) {
				if err := servePreviewFile(lw, r, candidate, fsys, preview.Options{SideMenu: buildSideMenu(r, requestedPath, candidate, fsys)}); err != nil {
					serveFileError(lw, r, "Serve error", err, directoryRecoveryLinks(strategy))
				}
				return
			}
			if err := serveFile(lw, r, candidate, fsys, config); err != nil {
				serveFileError(lw, r, "Serve error", err, directoryRecoveryLinks(strategy))
			}
			return
		}
	}

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

func serveFile(lw http.ResponseWriter, r *http.Request, requestedPath string, fsys fs.FS, config serveConfig) error {
	content, err := fs.ReadFile(fsys, requestedPath)
	if err != nil {
		return err
	}

	if shouldRenderPreview(r, requestedPath, config.Mode) {
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

func detectContentType(node *servfs.FsNode) (string, error) {
	if node.Info.IsDir() {
		return "", errors.New("not implemented")
	}

	return mime.TypeByExtension(filepath.Ext(node.Path)), nil
}
