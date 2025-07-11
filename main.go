package main

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v3"
)

func main() {
	app := &cli.Command{
		Name:  "serv",
		Usage: "Serve static files",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Value:   "8080",
				Usage:   "HTTP server port",
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
	file := cmd.StringArg("file")
	port := cmd.String("port")

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
