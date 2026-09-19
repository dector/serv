package netutil

import (
	"net"
	"testing"
	"time"
)

func TestWaitForTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	if err := WaitForTCP(listener.Addr().String(), 200*time.Millisecond); err != nil {
		t.Fatalf("WaitForTCP() returned error for listening socket: %v", err)
	}
}

func TestValidateTCPPort(t *testing.T) {
	tests := []struct {
		value   string
		want    string
		wantErr bool
	}{
		{value: "8080", want: "8080"},
		{value: "443", want: "443"},
		{value: "0", wantErr: true},
		{value: "70000", wantErr: true},
		{value: "abc", wantErr: true},
	}

	for _, tt := range tests {
		got, err := ValidateTCPPort(tt.value)
		if (err != nil) != tt.wantErr {
			t.Fatalf("ValidateTCPPort(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
		}
		if got != tt.want {
			t.Fatalf("ValidateTCPPort(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}
