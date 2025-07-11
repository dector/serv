package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v3"
)

const defaultPort = 8080

func main() {
	app := &cli.Command{
		Name:  "serv",
		Usage: "Serve static files",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Value:   fmt.Sprintf("%d", defaultPort),
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
		fmt.Println("0.1.0-00")
		return nil
	}

	file := cmd.StringArg("file")
	port := choosePort(cmd.String("port"))

	file, err := filepath.Abs(file)
	if err != nil {
		return errors.Wrap(err, "failed to get absolute path")
	}

	if _, err := os.Stat(file); os.IsNotExist(err) {
		return errors.Errorf("file does not exist: %s", file)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		contentType, err := detectContentType(file)

		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", contentType)
		http.ServeFile(w, r, file)
	})

	fmt.Printf("Serving `%s` on http://localhost:%s\n", file, port)
	return http.ListenAndServe(":"+port, nil)
}

func detectContentType(file string) (string, error) {
	info, err := os.Stat(file)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		return "", errors.New("not implemented")
	}

	return mime.TypeByExtension(filepath.Ext(file)), nil
}

func choosePort(port string) string {
	portNum, err := strconv.Atoi(port)
	if err != nil {
		fmt.Printf("Warning: invalid port '%s', using default port %d\n", port, defaultPort)
		portNum = defaultPort
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
