package stats

import (
	"net/http"
	"strconv"
)

// NewStatHandler returns an http handler for stats.
func NewStatHandler(scope Scope, handler http.Handler) http.Handler {
	ret := statHandler{}
	ret.scope = scope
	ret.delegate = handler
	ret.timer = ret.scope.NewTimer("rq_time_us")
	return &ret
}

type statHandler struct {
	prefix   string
	scope    Scope
	delegate http.Handler
	timer    Timer
}

type statResponseWriter struct {
	handler  *statHandler
	delegate http.ResponseWriter
	span     Timespan
}

func (h *statHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rw := &statResponseWriter{h, w, h.timer.AllocateSpan()}
	h.delegate.ServeHTTP(rw, r)
	rw.span.Complete()
}

func (h *statResponseWriter) Header() http.Header {
	return h.delegate.Header()
}

func (h *statResponseWriter) Write(b []byte) (int, error) {
	return h.delegate.Write(b)
}

func (h *statResponseWriter) WriteHeader(code int) {
	h.handler.scope.NewCounter(strconv.Itoa(code)).Inc()
	h.delegate.WriteHeader(code)
}

func (h *statResponseWriter) Flush() {
	if flusher, ok := h.delegate.(http.Flusher); ok {
		flusher.Flush()
	}
}
