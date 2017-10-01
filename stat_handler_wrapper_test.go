// +build go1.8

package stats

import (
	"fmt"
	"net/http"
	"testing"
)

func TestHTTPHandler_WrapResponse(t *testing.T) {
	t.Parallel()

	tests := []http.ResponseWriter{
		struct {
			http.ResponseWriter
			http.Flusher
			http.Hijacker
			http.Pusher
			http.CloseNotifier
		}{},
		struct {
			http.ResponseWriter
			http.Flusher
			http.Hijacker
			http.Pusher
		}{},
		struct {
			http.ResponseWriter
			http.Flusher
			http.Hijacker
			http.CloseNotifier
		}{},
		struct {
			http.ResponseWriter
			http.Flusher
			http.Hijacker
		}{},
		struct {
			http.ResponseWriter
			http.Flusher
			http.Pusher
			http.CloseNotifier
		}{},
		struct {
			http.ResponseWriter
			http.Flusher
			http.Pusher
		}{},
		struct {
			http.ResponseWriter
			http.Flusher
			http.CloseNotifier
		}{},
		struct {
			http.ResponseWriter
			http.Flusher
		}{},
		struct {
			http.ResponseWriter
			http.Hijacker
			http.Pusher
			http.CloseNotifier
		}{},
		struct {
			http.ResponseWriter
			http.Hijacker
			http.Pusher
		}{},
		struct {
			http.ResponseWriter
			http.Hijacker
			http.CloseNotifier
		}{},
		struct {
			http.ResponseWriter
			http.Hijacker
		}{},
		struct {
			http.ResponseWriter
			http.Pusher
			http.CloseNotifier
		}{},
		struct {
			http.ResponseWriter
			http.Pusher
		}{},
		struct {
			http.ResponseWriter
			http.CloseNotifier
		}{},
		struct{ http.ResponseWriter }{},
	}

	h := NewStatHandler(
		NewStore(NewNullSink(), false),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).(*httpHandler)

	for i, test := range tests {
		tc := test
		t.Run(fmt.Sprint("test:", i), func(t *testing.T) {
			t.Parallel()

			_, canFlush := tc.(http.Flusher)
			_, canHijack := tc.(http.Hijacker)
			_, canPush := tc.(http.Pusher)
			_, canNotify := tc.(http.CloseNotifier)

			rw := h.wrapResponse(tc)

			if _, ok := rw.(http.Flusher); ok != canFlush {
				t.Errorf("Flusher: wanted %t", canFlush)
			}
			if _, ok := rw.(http.Hijacker); ok != canHijack {
				t.Errorf("Hijacker: wanted %t", canHijack)
			}
			if _, ok := rw.(http.Pusher); ok != canPush {
				t.Errorf("Pusher: wanted %t", canPush)
			}
			if _, ok := rw.(http.CloseNotifier); ok != canNotify {
				t.Errorf("CloseNotifier: wanted %t", canNotify)
			}
		})
	}
}
