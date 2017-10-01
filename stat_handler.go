package stats

import (
	"net/http"
	"strconv"
	"sync"
)

const requestTimer = "rq_time_us"

type httpHandler struct {
	prefix   string
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

func (h *httpHandler) wrapResponse(w http.ResponseWriter) http.ResponseWriter {
	rw := &responseWriter{
		ResponseWriter: w,
		handler:        h,
	}

	flusher, canFlush := w.(http.Flusher)
	hijacker, canHijack := w.(http.Hijacker)
	pusher, canPush := w.(http.Pusher)
	closeNotifier, canNotify := w.(http.CloseNotifier)

	if canFlush && canHijack && canPush && canNotify {
		return struct {
			http.ResponseWriter
			http.Flusher
			http.Hijacker
			http.Pusher
			http.CloseNotifier
		}{rw, flusher, hijacker, pusher, closeNotifier}
	} else if canFlush && canHijack && canPush {
		return struct {
			http.ResponseWriter
			http.Flusher
			http.Hijacker
			http.Pusher
		}{rw, flusher, hijacker, pusher}
	} else if canFlush && canHijack && canNotify {
		return struct {
			http.ResponseWriter
			http.Flusher
			http.Hijacker
			http.CloseNotifier
		}{rw, flusher, hijacker, closeNotifier}
	} else if canFlush && canPush && canNotify {
		return struct {
			http.ResponseWriter
			http.Flusher
			http.Pusher
			http.CloseNotifier
		}{rw, flusher, pusher, closeNotifier}
	} else if canHijack && canPush && canNotify {
		return struct {
			http.ResponseWriter
			http.Hijacker
			http.Pusher
			http.CloseNotifier
		}{rw, hijacker, pusher, closeNotifier}
	} else if canFlush && canHijack {
		return struct {
			http.ResponseWriter
			http.Flusher
			http.Hijacker
		}{rw, flusher, hijacker}
	} else if canFlush && canPush {
		return struct {
			http.ResponseWriter
			http.Flusher
			http.Pusher
		}{rw, flusher, pusher}
	} else if canFlush && canNotify {
		return struct {
			http.ResponseWriter
			http.Flusher
			http.CloseNotifier
		}{rw, flusher, closeNotifier}
	} else if canHijack && canPush {
		return struct {
			http.ResponseWriter
			http.Hijacker
			http.Pusher
		}{rw, hijacker, pusher}
	} else if canHijack && canNotify {
		return struct {
			http.ResponseWriter
			http.Hijacker
			http.CloseNotifier
		}{rw, hijacker, closeNotifier}
	} else if canPush && canNotify {
		return struct {
			http.ResponseWriter
			http.Pusher
			http.CloseNotifier
		}{rw, pusher, closeNotifier}
	} else if canFlush {
		return struct {
			http.ResponseWriter
			http.Flusher
		}{rw, flusher}
	} else if canHijack {
		return struct {
			http.ResponseWriter
			http.Hijacker
		}{rw, hijacker}
	} else if canPush {
		return struct {
			http.ResponseWriter
			http.Pusher
		}{rw, pusher}
	} else if canNotify {
		return struct {
			http.ResponseWriter
			http.CloseNotifier
		}{rw, closeNotifier}
	}

	return rw
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	span := h.timer.AllocateSpan()
	h.delegate.ServeHTTP(h.wrapResponse(w), r)
	span.Complete()
}

type responseWriter struct {
	http.ResponseWriter

	headerWritten bool
	span          Timespan
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
