package listen

import (
	"strconv"
	"testing"

	"dector.space/go/nettw"
)

func TestDetectExplicitBind(t *testing.T) {
	for _, bind := range []string{"", "all", "0.0.0.0", "127.0.0.1", "192.0.2.1", "::"} {
		for _, tailscale := range []bool{false, true} {
			t.Run(bind+"/tailscale="+strconv.FormatBool(tailscale), func(t *testing.T) {
				host := bind
				if host == "" || host == "all" {
					host = "0.0.0.0"
				}
				available := func(h, p string) bool { return h == host && p == "8080" }
				picker := func(string, string, bool) (nettw.Port, error) {
					t.Fatal("requested port is available on BIND host")
					return nettw.Port{}, nil
				}
				got, err := Detect("8080", true, "/root", tailscale, &bind, available, picker)
				if err != nil {
					t.Fatal(err)
				}
				readyHost := host
				if host == "0.0.0.0" {
					readyHost = "127.0.0.1"
				} else if host == "::" {
					readyHost = "::1"
				}
				if got.BindAddr != Addr(host, "8080") || got.ReadyAddr != Addr(readyHost, "8080") {
					t.Fatalf("unexpected config: %#v", got)
				}
			})
		}
	}
}

func TestDetectExplicitBindDoesNotFallBackToLocalhost(t *testing.T) {
	bind := "all"
	available := func(host, port string) bool { return host == "127.0.0.1" }
	picker := func(string, string, bool) (nettw.Port, error) {
		return nettw.Port{Str: "12345"}, nil
	}
	if _, err := Detect("8080", false, "/root", false, &bind, available, picker); err == nil {
		t.Fatal("expected error instead of localhost fallback")
	}
}
