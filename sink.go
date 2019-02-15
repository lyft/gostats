package stats

import (
	"bufio"
	"io"
	"math"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	DefaultFlushInterval = time.Second
	DefaultBufferSize    = 32 * 1024
)

// A Sink is used by a Store to flush its data.
// These functions may buffer the given data.
type Sink interface {
	FlushCounter(name string, value uint64)
	FlushGauge(name string, value uint64)
	FlushTimer(name string, value float64)
}

// FlushableSink is an extension of Sink that provides a Flush() function that
// will flush any buffered stats to the underlying store.
type FlushableSink interface {
	Sink
	Flush()
}

type flushableSink struct {
	mu       sync.Mutex
	buf      *bufio.Writer
	w        io.Writer
	interval time.Duration
}

func NewFlushableSink(w io.Writer) FlushableSink {
	s := &flushableSink{
		buf:      bufio.NewWriterSize(w, DefaultBufferSize),
		w:        w,
		interval: DefaultFlushInterval,
	}
	go s.run()
	return s
}

func (s *flushableSink) run() {
	if s.interval == 0 {
		return // don't flush
	}
	tick := time.NewTicker(s.interval)
	for range tick.C {
		s.Flush() // TODO: log error
	}
}

func (s *flushableSink) Write(b []byte) (int, error) {
	s.mu.Lock()
	// do not write partial stats
	if s.buf.Available() < len(b) && s.buf.Buffered() != 0 {
		if err := s.buf.Flush(); err != nil {
			s.mu.Unlock()
			return 0, err // TODO: reset?
		}
	}
	n, err := s.buf.Write(b)
	s.mu.Unlock()
	return n, err
}

func (s *flushableSink) Flush() {
	s.mu.Lock()
	s.buf.Flush() // WARN: return or log error
	s.mu.Unlock()
}

func (s *flushableSink) flushUint64(name, suffix string, u uint64) error {
	b := ppFree.Get().(*buffer)

	b.WriteString(name)
	b.WriteByte(':')
	b.WriteUnit64(u)
	b.WriteString(suffix)

	s.mu.Lock()
	_, err := s.buf.Write(*b)
	s.mu.Unlock()

	*b = (*b)[:0]
	ppFree.Put(b)
	return err // TODO (CEV): don't flush - use Flush() instead
}

func (s *flushableSink) flushFloat64(name, suffix string, f float64) error {
	b := ppFree.Get().(*buffer)

	b.WriteString(name)
	b.WriteByte(':')
	b.WriteFloat64(f)
	b.WriteString(suffix)

	s.mu.Lock()
	_, err := s.buf.Write(*b)
	s.mu.Unlock()

	*b = (*b)[:0]
	ppFree.Put(b)
	return err // TODO (CEV): don't flush - use Flush() instead
}

func (s *flushableSink) FlushCounter(name string, value uint64) {
	s.flushUint64(name, "|c\n", value)
}

func (s *flushableSink) FlushGauge(name string, value uint64) {
	s.flushUint64(name, "|g\n", value)
}

func (s *flushableSink) FlushTimer(name string, value float64) {
	const MaxUint64 float64 = 1<<64 - 1

	// Since we mistakenly use floating point values to represent time
	// durations this method is often passed an integer encoded as a
	// float. Formatting integers is much faster (>2x) than formatting
	// floats so use integer formatting whenever possible.
	//
	if 0 <= value && value < MaxUint64 && math.Trunc(value) == value {
		s.flushUint64(name, "|ms\n", uint64(value))
	} else {
		s.flushFloat64(name, "|ms\n", value)
	}
}

//////////////////////////////////////////////////////////////

// TODO (CEV): rename once we remove bufferPool
var ppFree = sync.Pool{
	New: func() interface{} {
		b := make(buffer, 0, 128)
		return &b
	},
}

// Use a fast and simple buffer for constructing statsd messages
type buffer []byte

func (b *buffer) Write(p []byte) {
	*b = append(*b, p...)
}

func (b *buffer) WriteString(s string) {
	*b = append(*b, s...)
}

func (b *buffer) WriteByte(c byte) {
	*b = append(*b, c)
}

func (b *buffer) WriteUnit64(val uint64) {
	var buf [20]byte // big enough for 64bit value base 10
	i := len(buf) - 1
	for val >= 10 {
		q := val / 10
		buf[i] = byte('0' + val - q*10)
		i--
		val = q
	}
	buf[i] = byte('0' + val)
	*b = append(*b, buf[i:]...)
}

func (b *buffer) WriteFloat64(val float64) {
	// TODO: trim precision to 6 ???
	// CEV: fmt uses a precision of 6 by default
	*b = strconv.AppendFloat(*b, val, 'f', -1, 64)
}

//////////////////////////////////////////////////////////////

// TODO: add optional logger
// WARN: if using UDP we need to limit the size of writes
type netWriter struct {
	conn    net.Conn
	mu      sync.Mutex
	network string
	raddr   string
}

// TODO: maybe rename to 'dial'
func newNetWriter(network, raddr string) (*netWriter, error) {
	w := &netWriter{
		network: network,
		raddr:   raddr,
	}
	if err := w.connect(); err != nil {
		return nil, err
	}
	return w, nil
}

const (
	DialTimeout  = time.Second
	WriteTimeout = time.Millisecond * 100
	MaxRetries   = 10
)

func (w *netWriter) connect() error {
	if w.conn != nil {
		w.conn.Close()
		w.conn = nil
	}
	c, err := net.DialTimeout(w.network, w.raddr, DialTimeout)
	if err != nil {
		return err
	}
	w.conn = c
	return nil
}

func (w *netWriter) Close() (err error) {
	w.mu.Lock()
	if w.conn != nil {
		err = w.conn.Close()
		w.conn = nil
	}
	w.mu.Unlock()
	return
}

func (w *netWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	n, err := w.writeAndRetry(b, 0)
	w.mu.Unlock()
	return n, err
}

func (w *netWriter) writeTimeout(b []byte) (int, error) {
	to := time.Now().Add(WriteTimeout)
	if err := w.conn.SetWriteDeadline(to); err != nil {
		return 0, err // WARN: what to do here?
	}
	return w.conn.Write(b)
}

func (w *netWriter) writeAndRetry(b []byte, retryCount int) (n int, err error) {
	if w.conn != nil {
		if n, err = w.writeTimeout(b); err == nil {
			return
		}
		// if we made progress, try one more write
		if (0 < n && n < len(b)) && isTimeoutError(err) {
			if b, n, err = w.handleTimeout(b, n); err == nil {
				return
			}
		}
		// prevent an infinite retry loop
		retryCount++
		if retryCount >= MaxRetries {
			return
		}
		// TODO: backoff here ???
	}
	if err = w.connect(); err != nil {
		return
	}
	return w.writeAndRetry(b, retryCount)
}

// TODO: see if this is worth having - might be better to just
// reset the connection or return the Timeout error.
func (w *netWriter) handleTimeout(b []byte, n int) ([]byte, int, error) {
	p := b[n:]                   // prevent duplicate data from being written
	no, err := w.writeTimeout(p) // try again
	if 0 < no && no < len(p) {
		p = p[no:] // trim successfully written data
	}
	return p, n + no, err
}

func isTimeoutError(err error) bool {
	v, ok := err.(*net.OpError)
	return ok && v.Timeout()
}
