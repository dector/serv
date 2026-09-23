package main

import (
	"fmt"
	"strconv"
	"strings"

	servnet "github.com/dector/serv/internal/netutil"
)

const defaultExposeValue = "__serv_expose_tailscale_default__"

type exposeConfig struct {
	Enabled bool
	Port    string
}

func parseExposeConfig(isSet bool, value, defaultPort string) (exposeConfig, error) {
	if !isSet {
		return exposeConfig{}, nil
	}
	if value == "" || value == defaultExposeValue {
		value = defaultPort
	}
	port, err := servnet.ValidateTCPPort(value)
	if err != nil {
		return exposeConfig{}, fmt.Errorf("invalid --expose-tailscale port: %w", err)
	}
	return exposeConfig{Enabled: true, Port: port}, nil
}

// normalizeArgs keeps positional arguments after an optional -T flag.
func normalizeArgs(args []string) []string {
	out := make([]string, 0, len(args)+1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		out = append(out, arg)
		switch {
		case arg == "-T" || arg == "--expose-tailscale":
			if i+1 >= len(args) || !looksLikePort(args[i+1]) {
				out = append(out, defaultExposeValue)
			}
		case strings.HasPrefix(arg, "--expose-tailscale="):
			if strings.TrimPrefix(arg, "--expose-tailscale=") == "" {
				out[len(out)-1] = "--expose-tailscale=" + defaultExposeValue
			}
		}
	}
	return out
}

func looksLikePort(value string) bool { _, err := strconv.Atoi(value); return err == nil }
