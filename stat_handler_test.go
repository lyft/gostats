package stats

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/lyft/gostats/mock"
)

func TestHttpHandler_ServeHTTP(t *testing.T) {
	Parallel(t)

	sink := mock.NewSink()
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

	sink.AssertTimerCallCount(t, requestTimer, 2)
	sink.AssertCounterEquals(t, "200", 1)
	sink.AssertCounterEquals(t, "404", 1)
}
