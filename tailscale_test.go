package main

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestNormalizeExposeTailscaleArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "long flag without value keeps path argument",
			args: []string{"serv", "--expose-tailscale", "."},
			want: []string{"serv", "--expose-tailscale", exposeTailscaleDefaultValue, "."},
		},
		{
			name: "short flag without value keeps path argument",
			args: []string{"serv", "-T", "."},
			want: []string{"serv", "-T", exposeTailscaleDefaultValue, "."},
		},
		{
			name: "short flag with port value",
			args: []string{"serv", "-T", "443", "."},
			want: []string{"serv", "-T", "443", "."},
		},
		{
			name: "negative numeric value is kept for validation",
			args: []string{"serv", "-T", "-1", "."},
			want: []string{"serv", "-T", "-1", "."},
		},
		{
			name: "long flag with equals value",
			args: []string{"serv", "--expose-tailscale=8443", "."},
			want: []string{"serv", "--expose-tailscale=8443", "."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeExposeTailscaleArgs(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizeExposeTailscaleArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseExposeTailscaleConfig(t *testing.T) {
	tests := []struct {
		name        string
		isSet       bool
		value       string
		defaultPort string
		want        tailscaleConfig
		wantErr     bool
	}{
		{name: "disabled", defaultPort: "50000", want: tailscaleConfig{}},
		{name: "default port", isSet: true, value: exposeTailscaleDefaultValue, defaultPort: "50000", want: tailscaleConfig{Enabled: true, Port: "50000"}},
		{name: "explicit port", isSet: true, value: "443", defaultPort: "50000", want: tailscaleConfig{Enabled: true, Port: "443"}},
		{name: "invalid port", isSet: true, value: "70000", defaultPort: "50000", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExposeTailscaleConfig(tt.isSet, tt.value, tt.defaultPort)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseExposeTailscaleConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("parseExposeTailscaleConfig() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestTailscaleStatusHasHTTPSPort(t *testing.T) {
	status := []byte(`{
		"ServeConfig": {
			"Web": {
				"example.tailnet.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:50000"}}},
				"example.tailnet.ts.net:8443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:50001"}}}
			}
		}
	}`)

	if !tailscaleStatusHasHTTPSPort(status, "443") {
		t.Fatal("expected status to include HTTPS port 443")
	}
	if tailscaleStatusHasHTTPSPort(status, "50000") {
		t.Fatal("backend local port should not be treated as occupied Tailscale HTTPS port")
	}
}

type checkRunner struct {
	lookPathErr error
	status      []byte
	statusErr   error
}

func (r checkRunner) LookPath() error                            { return r.lookPathErr }
func (r checkRunner) StatusJSON(context.Context) ([]byte, error) { return r.status, r.statusErr }
func (r checkRunner) StartServe(context.Context, string, string, io.Writer, io.Writer) (tailscaleProcess, error) {
	return nil, nil
}

func TestTailscaleServeArgs(t *testing.T) {
	want := []string{"serve", "--yes", "--https", "443", "http://127.0.0.1:50000"}
	got := tailscaleServeArgs("443", "50000")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tailscaleServeArgs() = %#v, want %#v", got, want)
	}
}

func TestParseTailscaleHTTPSURL(t *testing.T) {
	output := `Available within your tailnet:

https://factory.chicken-matrix.ts.net:60894/
|-- proxy http://127.0.0.1:60894`

	want := "https://factory.chicken-matrix.ts.net:60894"
	if got := parseTailscaleHTTPSURL(output); got != want {
		t.Fatalf("parseTailscaleHTTPSURL() = %q, want %q", got, want)
	}
}

func TestTailscaleURLWriterReportsFirstURL(t *testing.T) {
	writer := newTailscaleURLWriter()
	_, _ = writer.Write([]byte("Available within your tailnet:\n"))
	_, _ = writer.Write([]byte("https://factory.chicken-matrix.ts.net:60894/\n"))

	select {
	case got := <-writer.URL():
		want := "https://factory.chicken-matrix.ts.net:60894"
		if got != want {
			t.Fatalf("URL() = %q, want %q", got, want)
		}
	default:
		t.Fatal("expected URL writer to report parsed URL")
	}
}

func TestCheckTailscaleReady(t *testing.T) {
	if err := checkTailscaleReady(context.Background(), checkRunner{lookPathErr: errors.New("missing")}, "443"); err == nil {
		t.Fatal("expected missing tailscale to fail")
	}

	if err := checkTailscaleReady(context.Background(), checkRunner{status: []byte(`{"ServeConfig":{"Web":{"host.ts.net:443":{}}}}`)}, "443"); err == nil {
		t.Fatal("expected configured Tailscale port to fail")
	}

	if err := checkTailscaleReady(context.Background(), checkRunner{status: []byte(`no serve config`), statusErr: errors.New("exit 1")}, "443"); err != nil {
		t.Fatalf("no serve config should be allowed: %v", err)
	}
}
