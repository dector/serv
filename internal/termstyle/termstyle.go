// Package termstyle holds ANSI terminal colors and color helpers used by the
// CLI. It is intentionally isolated from the HTML theme.
package termstyle

import "strings"

const (
	Reset      = "\x1b[0m"
	Bold       = "\x1b[1m"
	Green      = "\x1b[32m"
	MutedGreen = "\x1b[38;5;65m"
	BrightCyan = "\x1b[96m"
	White      = "\x1b[37m"
	Underline  = "\x1b[4m"
)

// ShouldUseColor reports whether colored output should be used. It honors
// NO_COLOR, TERM=dumb and FORCE_COLOR before falling back to the terminal
// check.
func ShouldUseColor(stdoutIsTerminal bool, lookupEnv func(string) (string, bool)) bool {
	if _, ok := lookupEnv("NO_COLOR"); ok {
		return false
	}
	if termValue, ok := lookupEnv("TERM"); ok && termValue == "dumb" {
		return false
	}
	if forceColor, ok := lookupEnv("FORCE_COLOR"); ok && forceColor != "" && forceColor != "0" {
		return true
	}
	return stdoutIsTerminal
}

// Colorize wraps value with the given ANSI codes and a reset suffix.
func Colorize(value string, codes ...string) string {
	return strings.Join(codes, "") + value + Reset
}
