package output

import (
	"strings"
	"testing"
)

func TestBrowserCommand(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		wantName string
		wantArgs []string
		wantOK   bool
	}{
		{name: "linux", goos: "linux", wantName: "xdg-open", wantArgs: []string{"http://localhost:8080"}, wantOK: true},
		{name: "darwin", goos: "darwin", wantName: "open", wantArgs: []string{"http://localhost:8080"}, wantOK: true},
		{name: "windows", goos: "windows", wantName: "rundll32", wantArgs: []string{"url.dll,FileProtocolHandler", "http://localhost:8080"}, wantOK: true},
		{name: "unsupported", goos: "plan9", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotArgs, gotOK := browserCommand(tt.goos, "http://localhost:8080")
			if gotOK != tt.wantOK || gotName != tt.wantName || strings.Join(gotArgs, "\x00") != strings.Join(tt.wantArgs, "\x00") {
				t.Fatalf("browserCommand() = (%q, %q, %v), want (%q, %q, %v)", gotName, gotArgs, gotOK, tt.wantName, tt.wantArgs, tt.wantOK)
			}
		})
	}
}
