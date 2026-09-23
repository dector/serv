package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dector/serv/internal/version"
)

func main() {
	version.Init()

	if err := newApp().Run(context.Background(), normalizeArgs(os.Args)); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		os.Exit(1)
	}
}
