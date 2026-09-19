package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dector/serv/internal/tailscale"
	"github.com/dector/serv/internal/version"
)

func main() {
	version.Init()

	if err := newApp().Run(context.Background(), tailscale.NormalizeArgs(os.Args)); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		os.Exit(1)
	}
}
