//go:build go1.8
// +build go1.8

package stats

import "net/http"

func (h *httpHandler) wrapResponse(w http.ResponseWriter) http.ResponseWriter {
	rw := &responseWriter{
		ResponseWriter: w,
		handler:        h,
	}

	flusher, canFlush := w.(http.Flusher)
	hijacker, canHijack := w.(http.Hijacker)
	pusher, canPush := w.(http.Pusher)

	//nolint:staticcheck
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
