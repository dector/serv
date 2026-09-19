// Package output renders CLI status, banner, and browser launch helpers.
package output

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/dector/serv/internal/netutil"
	"github.com/dector/serv/internal/termstyle"
	"github.com/pkg/errors"
	"golang.org/x/term"
)

const servBanner = `
███████╗███████╗██████╗ ██╗   ██╗
██╔════╝██╔════╝██╔══██╗██║   ██║
███████╗█████╗  ██████╔╝██║   ██║
╚════██║██╔══╝  ██╔══██╗╚██╗ ██╔╝
███████║███████╗██║  ██║ ╚████╔╝
╚══════╝╚══════╝╚═╝  ╚═╝  ╚═══╝`

func printBanner(version string, useColor bool) {
	lines := strings.Split(servBanner, "\n")
	if len(lines) == 0 {
		return
	}

	if len(lines) > 1 {
		bannerHead := strings.Join(lines[:len(lines)-1], "\n")
		if useColor {
			bannerHead = termstyle.Colorize(bannerHead, termstyle.MutedGreen)
		}
		fmt.Println(bannerHead)
	}

	bannerTail := lines[len(lines)-1]
	versionText := fmt.Sprintf("v. %s", version)
	if useColor {
		bannerTail = termstyle.Colorize(bannerTail, termstyle.MutedGreen)
		versionText = termstyle.Colorize(versionText, termstyle.White)
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

// OpenBrowserWhenReady waits for the port to accept connections and then
// opens url in the default browser, logging failures without aborting.
func OpenBrowserWhenReady(port, url string) {
	go func() {
		if err := netutil.WaitForTCP(net.JoinHostPort("127.0.0.1", port), 2*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: server did not become ready for browser launch: %v\n", err)
			return
		}
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to open browser: %v\n", err)
		}
	}()
}

// ServedURL builds the local URL for a port.
func ServedURL(port string) string {
	return fmt.Sprintf("http://localhost:%s", port)
}

// PrintLaunchInfo prints the banner, served path, and local URL.
func PrintLaunchInfo(version, rootFile, port string) {
	useColor := termstyle.ShouldUseColor(term.IsTerminal(int(os.Stdout.Fd())), os.LookupEnv)

	printBanner(version, useColor)

	bullet := "●"
	pathText := rootFile
	if useColor {
		bullet = termstyle.Colorize(bullet, termstyle.Green)
		pathText = termstyle.Colorize(pathText, termstyle.Bold)
	}

	fmt.Printf("%s serving %s\n", bullet, pathText)
	PrintURL(ServedURL(port))
}

// PrintURL prints a URL using the terminal link styling.
func PrintURL(url string) {
	useColor := termstyle.ShouldUseColor(term.IsTerminal(int(os.Stdout.Fd())), os.LookupEnv)
	urlText := url
	if useColor {
		urlText = termstyle.Colorize(urlText, termstyle.BrightCyan, termstyle.Underline)
	}
	fmt.Printf("  %s\n", urlText)
}
