package main

import (
	"reflect"
	"testing"
)

func TestNormalizeArgs(t *testing.T) {
	for _, tt := range []struct{ args, want []string }{
		{[]string{"serv", "--expose-tailscale", "."}, []string{"serv", "--expose-tailscale", defaultExposeValue, "."}},
		{[]string{"serv", "-T", "."}, []string{"serv", "-T", defaultExposeValue, "."}},
		{[]string{"serv", "-T", "443", "."}, []string{"serv", "-T", "443", "."}},
		{[]string{"serv", "-T", "-1", "."}, []string{"serv", "-T", "-1", "."}},
		{[]string{"serv", "--expose-tailscale=8443", "."}, []string{"serv", "--expose-tailscale=8443", "."}},
	} {
		if got := normalizeArgs(tt.args); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("normalizeArgs(%v) = %v; want %v", tt.args, got, tt.want)
		}
	}
}
func TestParseExposeConfig(t *testing.T) {
	for _, tt := range []struct {
		set                bool
		value, defaultPort string
		want               exposeConfig
		wantErr            bool
	}{
		{false, "", "50000", exposeConfig{}, false},
		{true, defaultExposeValue, "50000", exposeConfig{true, "50000"}, false},
		{true, "443", "50000", exposeConfig{true, "443"}, false},
		{true, "70000", "50000", exposeConfig{}, true},
	} {
		got, err := parseExposeConfig(tt.set, tt.value, tt.defaultPort)
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("parseExposeConfig(%v,%q) = %+v, %v", tt.set, tt.value, got, err)
		}
	}
}
