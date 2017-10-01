package stats

import (
	"net/http"
	"testing"
)

func TestResponseWriter_InterfacePassthrough(t *testing.T) {
	t.Parallel()

	w := struct {
		http.ResponseWriter
		http.Flusher
		http.Hijacker
		http.Pusher
		http.CloseNotifier
	}{}

	var rw http.ResponseWriter = &responseWriter{ResponseWriter: w}

	if _, ok := rw.(http.Flusher); !ok {
		t.Fatal("stat responseWriter should pass through Flusher")
	}
}
