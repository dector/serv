package main

import "testing"

func TestShouldUseColor(t *testing.T) {
	tests := []struct {
		name             string
		stdoutIsTerminal bool
		env              map[string]string
		want             bool
	}{
		{
			name:             "terminal enables color",
			stdoutIsTerminal: true,
			want:             true,
		},
		{
			name:             "non-terminal disables color",
			stdoutIsTerminal: false,
			want:             false,
		},
		{
			name:             "force color enables color without terminal",
			stdoutIsTerminal: false,
			env:              map[string]string{"FORCE_COLOR": "1"},
			want:             true,
		},
		{
			name:             "force color zero does not enable color",
			stdoutIsTerminal: false,
			env:              map[string]string{"FORCE_COLOR": "0"},
			want:             false,
		},
		{
			name:             "no color wins over terminal",
			stdoutIsTerminal: true,
			env:              map[string]string{"NO_COLOR": ""},
			want:             false,
		},
		{
			name:             "no color wins over force color",
			stdoutIsTerminal: false,
			env:              map[string]string{"NO_COLOR": "", "FORCE_COLOR": "1"},
			want:             false,
		},
		{
			name:             "dumb terminal disables color",
			stdoutIsTerminal: true,
			env:              map[string]string{"TERM": "dumb"},
			want:             false,
		},
		{
			name:             "dumb terminal wins over force color",
			stdoutIsTerminal: false,
			env:              map[string]string{"TERM": "dumb", "FORCE_COLOR": "1"},
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUseColor(tt.stdoutIsTerminal, func(key string) (string, bool) {
				value, ok := tt.env[key]
				return value, ok
			})
			if got != tt.want {
				t.Fatalf("shouldUseColor() = %v, want %v", got, tt.want)
			}
		})
	}
}
