// +build !go1.8

package stats

import "net/http"

func (h *httpHandler) wrapResponse(w http.ResponseWriter) http.ResponseWriter {
	rw := &responseWriter{
		ResponseWriter: w,
		handler:        h,
	}

	flusher, canFlush := w.(http.Flusher)
	hijacker, canHijack := w.(http.Hijacker)
	closeNotifier, canNotify := w.(http.CloseNotifier)

	if canFlush && canHijack && canNotify {
		return struct {
			http.ResponseWriter
			http.Flusher
			http.Hijacker
			http.CloseNotifier
		}{rw, flusher, hijacker, closeNotifier}
	} else if canFlush && canHijack {
		return struct {
			http.ResponseWriter
			http.Flusher
			http.Hijacker
		}{rw, flusher, hijacker}
	} else if canFlush && canNotify {
		return struct {
			http.ResponseWriter
			http.Flusher
			http.CloseNotifier
		}{rw, flusher, closeNotifier}
	} else if canHijack && canNotify {
		return struct {
			http.ResponseWriter
			http.Hijacker
			http.CloseNotifier
		}{rw, hijacker, closeNotifier}
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
	} else if canNotify {
		return struct {
			http.ResponseWriter
			http.CloseNotifier
		}{rw, closeNotifier}
	}

	return rw
}
