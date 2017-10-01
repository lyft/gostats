package stats

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestHttpHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	sink := NewMockSink()
	store := NewStore(sink, false)

	h := NewStatHandler(
		store,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if code, err := strconv.Atoi(r.Header.Get("code")); err == nil {
				w.WriteHeader(code)
			}

			io.Copy(w, r.Body)
			r.Body.Close()
		})).(*httpHandler)

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		r, _ := http.NewRequest(http.MethodGet, "/", strings.NewReader("foo"))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		store.Flush()

		if w.Body.String() != "foo" {
			t.Errorf("wanted %q body, got %q", "foo", w.Body.String())
		}

		if w.Code != http.StatusOK {
			t.Errorf("wanted 200, got %d", w.Code)
		}

		wg.Done()
	}()

	go func() {
		r := httptest.NewRequest(http.MethodGet, "/", strings.NewReader("bar"))
		r.Header.Set("code", strconv.Itoa(http.StatusNotFound))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		store.Flush()

		if w.Body.String() != "bar" {
			t.Errorf("wanted %q body, got %q", "bar", w.Body.String())
		}

		if w.Code != http.StatusNotFound {
			t.Errorf("wanted 404, got %d", w.Code)
		}

		wg.Done()
	}()

	wg.Wait()

	timer, ok := sink.Timers[requestTimer]
	if !ok {
		t.Errorf("wanted a %q timer, none found", requestTimer)
	} else if timer != 2 {
		t.Error("wanted 2, got", timer)
	}

	c, ok := sink.Counters["200"]
	if !ok {
		t.Error("wanted a '200' counter, none found")
	} else if c != 1 {
		t.Error("wanted 1, got", c)
	}

	c, ok = sink.Counters["404"]
	if !ok {
		t.Error("wanted a '404' counter, none found")
	} else if c != 1 {
		t.Error("wanted 1, got", c)
	}
}

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
