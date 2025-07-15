package middleware

import (
	"fmt"
	"net/http"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	lw.statusCode = code
	lw.ResponseWriter.WriteHeader(code)
}

func logRequest(method string, statusCode int, path string) {
	var color string
	if statusCode >= 200 && statusCode < 400 {
		color = "\033[48;5;22m" // Dark green background
	} else {
		color = "\033[48;5;52m" // Dark red background
	}
	reset := "\033[0m"

	fmt.Printf("%s %s %s %d %s\n", color, method, reset, statusCode, path)
}

func WithLogging(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lw := &loggingResponseWriter{ResponseWriter: w, statusCode: 200}

		defer func() {
			logRequest(r.Method, lw.statusCode, r.URL.Path)
		}()

		handler(lw, r)
	}
}
