package netutil

import (
	"net"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

// WaitForTCP blocks until addr accepts a TCP connection or timeout elapses.
func WaitForTCP(addr string, timeout time.Duration) error {
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

// ValidateTCPPort normalizes a TCP port string and rejects values outside
// 1-65535.
func ValidateTCPPort(value string) (string, error) {
	port, err := strconv.Atoi(value)
	if err != nil {
		return "", err
	}
	if port < 1 || port > 65535 {
		return "", errors.Errorf("port %d out of range 1-65535", port)
	}
	return strconv.Itoa(port), nil
}
