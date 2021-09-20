package stats

import (
	"net/http"
	"strconv"
	"sync"
)

const requestTimer = "rq_time_us"

type httpHandler struct {
	scope    Scope
	delegate http.Handler

	timer Timer

	codes    map[int]Counter
	codesMtx sync.RWMutex
}

// NewStatHandler returns an http handler for stats.
func NewStatHandler(scope Scope, handler http.Handler) http.Handler {
	return &httpHandler{
		scope:    scope,
		delegate: handler,
		timer:    scope.NewTimer(requestTimer),
		codes:    map[int]Counter{},
	}
}

func (h *httpHandler) counter(code int) Counter {
	h.codesMtx.RLock()
	c := h.codes[code]
	h.codesMtx.RUnlock()

	if c != nil {
		return c
	}

	h.codesMtx.Lock()
	if c = h.codes[code]; c == nil {
		c = h.scope.NewCounter(strconv.Itoa(code))
		h.codes[code] = c
	}
	h.codesMtx.Unlock()

	return c
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	span := h.timer.AllocateSpan()
	h.delegate.ServeHTTP(h.wrapResponse(w), r)
	span.Complete()
}

type responseWriter struct {
	http.ResponseWriter

	headerWritten bool
	handler       *httpHandler
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.headerWritten {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.headerWritten {
		return
	}

	rw.headerWritten = true
	rw.handler.counter(code).Inc()
	rw.ResponseWriter.WriteHeader(code)
}

var (
	_ http.Handler        = (*httpHandler)(nil)
	_ http.ResponseWriter = (*responseWriter)(nil)
)
