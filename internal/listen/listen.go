// Package listen chooses the local TCP port and bind address for the server.
package listen

import (
	"fmt"
	"net"

	"github.com/dector/nettw"
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

// PickLocalPort chooses a port, seeded by rootFile for stable results.
func PickLocalPort(requestedPort, rootFile string, exposeTailscale bool) (nettw.Port, error) {
	if exposeTailscale {
		return nettw.ParsePortOrPickAnother("random", nettw.WithIgnoreInvalidPort(true), nettw.WithSeed(rootFile), nettw.WithPortRange(49152, 65535))
	}
	return nettw.ParsePortOrPickAnother(requestedPort, nettw.WithSeed(rootFile))
}

// Detect resolves the effective listen configuration from the requested port.
func Detect(requestedPort string, portIsSet bool, rootFile string, exposeTailscale bool, available AvailabilityFunc, pickPort PickerFunc) (Config, error) {
	var port nettw.Port
	if !exposeTailscale {
		if parsedPort, err := servnet.ValidateTCPPort(requestedPort); err == nil && available("127.0.0.1", parsedPort) {
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
		if !available("127.0.0.1", parsedPort) {
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
	return Config{Port: port, BindAddr: bindAddr, ReadyAddr: readyAddr}, nil
}
