package listen

import (
	"net"
	"testing"

	"github.com/dector/nettw"
)

func TestDetectUsesRequestedPortWhenTailscaleBlocksAllInterfaces(t *testing.T) {
	available := func(host, port string) bool {
		return host == "127.0.0.1" && (port == "8080" || port == "3000")
	}
	picker := func(string, string, bool) (nettw.Port, error) {
		t.Fatal("picker should not be called when requested port is available on localhost")
		return nettw.Port{}, nil
	}

	for _, requestedPort := range []string{"8080", "3000"} {
		t.Run(requestedPort, func(t *testing.T) {
			got, err := Detect(requestedPort, requestedPort == "3000", "/root", false, available, picker)
			if err != nil {
				t.Fatalf("Detect() error = %v", err)
			}
			if got.Port.Str != requestedPort {
				t.Fatalf("port = %q, want %q", got.Port.Str, requestedPort)
			}
			if want := net.JoinHostPort("127.0.0.1", requestedPort); got.BindAddr != want || got.ReadyAddr != want {
				t.Fatalf("addresses = bind %q ready %q, want %q", got.BindAddr, got.ReadyAddr, want)
			}
		})
	}
}

func TestDetectPicksAnotherWhenLocalhostPortUnavailable(t *testing.T) {
	available := func(host, port string) bool {
		return port == "12345"
	}
	pickerCalled := false
	picker := func(requestedPort, rootFile string, exposeTailscale bool) (nettw.Port, error) {
		pickerCalled = true
		if requestedPort != "8080" || rootFile != "/root" || exposeTailscale {
			t.Fatalf("picker args = (%q, %q, %v), want (8080, /root, false)", requestedPort, rootFile, exposeTailscale)
		}
		return nettw.Port{Str: "12345"}, nil
	}

	got, err := Detect("8080", false, "/root", false, available, picker)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !pickerCalled {
		t.Fatal("expected picker to be called")
	}
	if got.Port.Str != "12345" || got.BindAddr != ":12345" {
		t.Fatalf("Detect() = %#v, want port 12345 bound on all interfaces", got)
	}
}

func TestDetectTailscaleExplicitPortRequiresLocalhostAvailability(t *testing.T) {
	available := func(string, string) bool { return false }
	picker := func(string, string, bool) (nettw.Port, error) {
		t.Fatal("picker should not be called for explicit Tailscale local port")
		return nettw.Port{}, nil
	}

	if _, err := Detect("3000", true, "/root", true, available, picker); err == nil {
		t.Fatal("expected unavailable explicit Tailscale local port to fail")
	}
}

func TestDetectTailscaleWithoutPortPicksRandomHighPort(t *testing.T) {
	available := func(host, port string) bool { return host == "127.0.0.1" && port == "55555" }
	picker := func(requestedPort, rootFile string, exposeTailscale bool) (nettw.Port, error) {
		if requestedPort != "random" || rootFile != "/root" || !exposeTailscale {
			t.Fatalf("picker args = (%q, %q, %v), want (random, /root, true)", requestedPort, rootFile, exposeTailscale)
		}
		return nettw.Port{Str: "55555"}, nil
	}

	got, err := Detect("8080", false, "/root", true, available, picker)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if got.Port.Str != "55555" || got.BindAddr != net.JoinHostPort("127.0.0.1", "55555") {
		t.Fatalf("Detect() = %#v, want Tailscale localhost bind on picked port", got)
	}
}
