// Package listen chooses the local TCP port and bind address for the server.
package listen

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"dector.space/go/nettw"
	servnet "github.com/dector/serv/internal/netutil"
	"github.com/pkg/errors"
)

// Config is the resolved listen setup.
type Config struct {
	Port      nettw.Port
	BindAddr  string
	ReadyAddr string
}

// AvailabilityFunc reports whether a port can be bound on a host.
type AvailabilityFunc func(host, port string) bool

// PickerFunc picks a port when the requested one cannot be used.
type PickerFunc func(requestedPort, rootFile string, exposeTailscale bool) (nettw.Port, error)

// Addr joins a host and port into a listen address. An empty host listens on
// all interfaces.
func Addr(host, port string) string {
	if host == "" {
		return ":" + port
	}
	return net.JoinHostPort(host, port)
}

// IsTCPPortAvailableOnHost reports whether host can bind port.
func IsTCPPortAvailableOnHost(host, port string) bool {
	ln, err := net.Listen("tcp", Addr(host, port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// PickLocalPort chooses a stable port, grouping paths in a repo by leading digits.
func PickLocalPort(requestedPort, rootFile string, exposeTailscale bool) (nettw.Port, error) {
	seed := portSeed(rootFile)
	if exposeTailscale {
		return nettw.ParsePortOrPickAnother("random", nettw.WithIgnoreInvalidPort(true), seed, nettw.WithPortRange(49152, 65535))
	}
	return nettw.ParsePortOrPickAnother(requestedPort, seed)
}

// portSeed finds the nearest .git marker (directory or worktree file).
// Outside a repo, preserve the original single-path seed.
func portSeed(rootFile string) nettw.ParsePortOption {
	path, err := filepath.Abs(rootFile)
	if err != nil {
		return nettw.WithSeed(rootFile)
	}
	for dir := path; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			rel, err := filepath.Rel(dir, path)
			if err == nil {
				return nettw.WithSeedSegments(
					nettw.SeedSegment{Seed: dir},
					nettw.SeedSegment{Seed: rel, Digits: 2},
				)
			}
		}
		if filepath.Dir(dir) == dir {
			return nettw.WithSeed(rootFile)
		}
	}
}

// Detect resolves the effective listen configuration from the requested port.
// A non-nil bindHost overrides automatic binding; empty and "all" mean 0.0.0.0.
func Detect(requestedPort string, portIsSet bool, rootFile string, exposeTailscale bool, bindHost *string, available AvailabilityFunc, pickPort PickerFunc) (Config, error) {
	host := "127.0.0.1"
	if bindHost != nil {
		host = *bindHost
		if host == "" || host == "all" {
			host = "0.0.0.0"
		}
	}
	var port nettw.Port
	if !exposeTailscale {
		if parsedPort, err := servnet.ValidateTCPPort(requestedPort); err == nil && available(host, parsedPort) {
			port = nettw.Port{Str: parsedPort}
		} else {
			pickedPort, err := pickPort(requestedPort, rootFile, false)
			if err != nil {
				return Config{}, err
			}
			port = pickedPort
		}
	} else if portIsSet {
		parsedPort, err := servnet.ValidateTCPPort(requestedPort)
		if err != nil {
			return Config{}, fmt.Errorf("invalid --port: %w", err)
		}
		if !available(host, parsedPort) {
			return Config{}, errors.Errorf("local port %s is not available", parsedPort)
		}
		port = nettw.Port{Str: parsedPort}
	} else {
		pickedPort, err := pickPort("random", rootFile, true)
		if err != nil {
			return Config{}, err
		}
		port = pickedPort
	}

	readyAddr := net.JoinHostPort("127.0.0.1", port.Str)
	bindAddr := Addr("", port.Str)
	if exposeTailscale || !available("", port.Str) {
		bindAddr = readyAddr
	}
	if bindHost != nil {
		if !available(host, port.Str) {
			return Config{}, errors.Errorf("BIND address %s is not available", Addr(host, port.Str))
		}
		bindAddr = Addr(host, port.Str)
		if host == "::" {
			readyAddr = Addr("::1", port.Str)
		} else if host != "0.0.0.0" {
			readyAddr = bindAddr
		}
	}
	return Config{Port: port, BindAddr: bindAddr, ReadyAddr: readyAddr}, nil
}
