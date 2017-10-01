package stats

import (
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
