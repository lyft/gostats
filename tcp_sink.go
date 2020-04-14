package stats

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/lyft/gostats/internal/bufferpool"
	"github.com/lyft/gostats/internal/stat"
	logger "github.com/sirupsen/logrus"
)

const (
	defaultRetryInterval = time.Second * 3
	defaultDialTimeout   = defaultRetryInterval / 2
	defaultWriteTimeout  = time.Second

	flushInterval           = time.Second
	logOnEveryNDroppedBytes = 1 << 15 // Log once per 32kb of dropped stats
	defaultBufferSize       = 1 << 16
	approxMaxMemBytes       = 1 << 22
	chanSize                = approxMaxMemBytes / defaultBufferSize
)

// An SinkOption configures a Sink.
type SinkOption interface {
	apply(*tcpStatsdSink)
}

// sinkOptionFunc wraps a func so it satisfies the Option interface.
type sinkOptionFunc func(*tcpStatsdSink)

func (f sinkOptionFunc) apply(sink *tcpStatsdSink) {
	f(sink)
}

// WithStatsdHost sets the host of the statsd sink otherwise the host is
// read from the environment variable "STATSD_HOST".
func WithStatsdHost(host string) SinkOption {
	return sinkOptionFunc(func(sink *tcpStatsdSink) {
		sink.statsdHost = host
	})
}

// WithStatsdPort sets the port of the statsd sink otherwise the port is
// read from the environment variable "STATSD_PORT".
func WithStatsdPort(port int) SinkOption {
	return sinkOptionFunc(func(sink *tcpStatsdSink) {
		sink.statsdPort = port
	})
}

// WithLogger configures the sink to use the provided logger otherwise
// the standard logrus logger is used.
func WithLogger(log *logger.Logger) SinkOption {
	// TODO (CEV): use the zap.Logger
	return sinkOptionFunc(func(sink *tcpStatsdSink) {
		sink.log = log
	})
}

// NewTCPStatsdSink returns a FlushableSink that is backed by a buffered writer
// and a separate goroutine that flushes those buffers to a statsd connection.
func NewTCPStatsdSink(opts ...SinkOption) FlushableSink {
	outc := make(chan *bytes.Buffer, chanSize) // TODO(btc): parameterize
	writer := sinkWriter{
		outc: outc,
	}
	// TODO (CEV): this auto loading from the env is bad and should be removed.
	conf := GetSettings()
	s := &tcpStatsdSink{
		outc: outc,
		// TODO(btc): parameterize size
		bufWriter: bufio.NewWriterSize(&writer, defaultBufferSize),
		// arbitrarily buffered
		doFlush: make(chan chan struct{}, 8),
		// CEV: default to the standard logger to match the legacy implementation.
		log:        logger.StandardLogger(),
		statsdHost: conf.StatsdHost,
		statsdPort: conf.StatsdPort,
	}
	for _, opt := range opts {
		opt.apply(s)
	}
	go s.run()
	return s
}

type tcpStatsdSink struct {
	// NEW
	stats <-chan stat.Stat
	bw    *bufio.Writer

	// KILL THIS
	outc chan *bytes.Buffer

	conn         net.Conn
	mu           sync.Mutex
	bufWriter    *bufio.Writer
	doFlush      chan chan struct{}
	droppedBytes uint64
	log          *logger.Logger
	statsdHost   string
	statsdPort   int
}

type sinkWriter struct {
	outc chan<- *bytes.Buffer
}

func (w *sinkWriter) Write(p []byte) (int, error) {
	n := len(p)
	dest := getBuffer()
	dest.Write(p)
	select {
	case w.outc <- dest:
		return n, nil
	default:
		return 0, fmt.Errorf("statsd channel full, dropping stats buffer with %d bytes", n)
	}
}

func (s *tcpStatsdSink) Flush() {
	if s.flush() != nil {
		return // nothing we can do
	}
	ch := make(chan struct{})
	s.doFlush <- ch
	<-ch
}

func (s *tcpStatsdSink) flush() error {
	s.mu.Lock()
	err := s.bufWriter.Flush()
	if err != nil {
		s.handleFlushError(err)
	}
	s.mu.Unlock()
	return err
}

func (s *tcpStatsdSink) drainFlushQueue() {
	// Limit the number of items we'll flush to prevent this from possibly
	// hanging when the flush channel is saturated with sends.
	doFlush := s.doFlush
	n := cap(doFlush) * 8
	for i := 0; i < n; i++ {
		select {
		case ch := <-doFlush:
			close(ch)
		default:
			return
		}
	}
}

// s.mu should be held
func (s *tcpStatsdSink) handleFlushErrorSize(err error, dropped int) {
	d := uint64(dropped)
	if (s.droppedBytes+d)%logOnEveryNDroppedBytes > s.droppedBytes%logOnEveryNDroppedBytes {
		s.log.WithField("total_dropped_bytes", s.droppedBytes+d).
			WithField("dropped_bytes", d).
			Error(err)
	}
	s.droppedBytes += d

	s.bufWriter.Reset(&sinkWriter{
		outc: s.outc,
	})
}

// s.mu should be held
func (s *tcpStatsdSink) handleFlushError(err error) {
	s.handleFlushErrorSize(err, s.bufWriter.Buffered())
}

func (s *tcpStatsdSink) writeBuffer(b *buffer) {
	s.mu.Lock()
	if s.bufWriter.Available() < b.Len() {
		if err := s.bufWriter.Flush(); err != nil {
			s.handleFlushError(err)
		}
		// If there is an error we reset the bufWriter so its
		// okay to attempt the write after the failed flush.
	}
	if _, err := s.bufWriter.Write(*b); err != nil {
		s.handleFlushError(err)
	}
	s.mu.Unlock()
}

func (s *tcpStatsdSink) flushUint64(name, suffix string, u uint64) {
	b := pbFree.Get().(*buffer)

	b.WriteString(name)
	b.WriteChar(':')
	b.WriteUnit64(u)
	b.WriteString(suffix)

	s.writeBuffer(b)

	b.Reset()
	pbFree.Put(b)
}

func (s *tcpStatsdSink) flushFloat64(name, suffix string, f float64) {
	b := pbFree.Get().(*buffer)

	b.WriteString(name)
	b.WriteChar(':')
	b.WriteFloat64(f)
	b.WriteString(suffix)

	s.writeBuffer(b)

	b.Reset()
	pbFree.Put(b)
}

func (s *tcpStatsdSink) FlushCounter(name string, value uint64) {
	s.flushUint64(name, "|c\n", value)
}

func (s *tcpStatsdSink) FlushGauge(name string, value uint64) {
	s.flushUint64(name, "|g\n", value)
}

func (s *tcpStatsdSink) FlushTimer(name string, value float64) {
	// Since we mistakenly use floating point values to represent time
	// durations this method is often passed an integer encoded as a
	// float. Formatting integers is much faster (>2x) than formatting
	// floats so use integer formatting whenever possible.
	//
	if 0 <= value && value < math.MaxUint64 && math.Trunc(value) == value {
		s.flushUint64(name, "|ms\n", uint64(value))
	} else {
		s.flushFloat64(name, "|ms\n", value)
	}
}

func (s *tcpStatsdSink) run() {
	addr := net.JoinHostPort(s.statsdHost, strconv.Itoa(s.statsdPort))

	var reconnectFailed bool // true if last reconnect failed

	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		if s.conn == nil {
			if err := s.connect(addr); err != nil {
				s.log.Warnf("statsd connection error: %s", err)

				// If the previous reconnect attempt failed, drain the flush
				// queue to prevent Flush() from blocking indefinitely.
				if reconnectFailed {
					s.drainFlushQueue()
				}
				reconnectFailed = true

				// TODO (CEV): don't sleep on the first retry
				time.Sleep(defaultRetryInterval)
				continue
			}
			reconnectFailed = false
		}

		select {
		case <-t.C:
			s.flush()
		case done := <-s.doFlush:
			// Only flush pending buffers, this prevents an issue where
			// continuous writes prevent the flush loop from exiting.
			//
			// If there is an error writeToConn() will set the conn to
			// nil thus breaking the loop.
			//
			n := len(s.outc)
			for i := 0; i < n && s.conn != nil; i++ {
				buf := <-s.outc
				s.writeToConn(buf)
				putBuffer(buf)
			}
			close(done)
		case stat := <-s.stats:
			b := bufferpool.Get()
			stat.Format(b)
			if s.bw.Available() < b.Len() {
				s.bw.Flush() // WARN: check error
			}
			s.bw.Write(b.Bytes()) // WARN: check error
			b.Free()
		// KILL
		case buf := <-s.outc:
			s.writeToConn(buf)
			putBuffer(buf)
		}
	}
}

// writeToConn writes the buffer to the underlying conn.  May only be called
// from run().
func (s *tcpStatsdSink) writeToConn(buf *bytes.Buffer) {
	len := buf.Len()

	// TODO (CEV): parameterize timeout
	s.conn.SetWriteDeadline(time.Now().Add(defaultWriteTimeout))
	_, err := buf.WriteTo(s.conn)
	s.conn.SetWriteDeadline(time.Time{}) // clear

	if err != nil {
		s.mu.Lock()
		s.handleFlushErrorSize(err, len)
		s.mu.Unlock()
		_ = s.conn.Close()
		s.conn = nil // this will break the loop
	}
}

func (s *tcpStatsdSink) connect(address string) error {
	// TODO (CEV): parameterize timeout
	conn, err := net.DialTimeout("tcp", address, defaultDialTimeout)
	if err != nil {
		return err
	}
	s.conn = conn
	if s.bw == nil {
		w := &timeoutWriter{
			timeout: defaultWriteTimeout,
			conn:    conn,
		}
		// WARN: make the size tunable
		s.bw = bufio.NewWriterSize(w, defaultBufferSize)
	}
	return err
}

var bufferPool sync.Pool

func getBuffer() *bytes.Buffer {
	if v := bufferPool.Get(); v != nil {
		b := v.(*bytes.Buffer)
		b.Reset()
		return b
	}
	return new(bytes.Buffer)
}

func putBuffer(b *bytes.Buffer) {
	bufferPool.Put(b)
}

// pbFree is the print buffer pool
var pbFree = sync.Pool{
	New: func() interface{} {
		b := make(buffer, 0, 128)
		return &b
	},
}

// Use a fast and simple buffer for constructing statsd messages
type buffer []byte

func (b *buffer) Len() int { return len(*b) }

func (b *buffer) Reset() { *b = (*b)[:0] }

func (b *buffer) Write(p []byte) {
	*b = append(*b, p...)
}

func (b *buffer) WriteString(s string) {
	*b = append(*b, s...)
}

// This is named WriteChar instead of WriteByte because the 'stdmethods' check
// of 'go vet' wants WriteByte to have the signature:
//
// 	func (b *buffer) WriteByte(c byte) error { ... }
//
func (b *buffer) WriteChar(c byte) {
	*b = append(*b, c)
}

func (b *buffer) WriteUnit64(val uint64) {
	*b = strconv.AppendUint(*b, val, 10)
}

func (b *buffer) WriteFloat64(val float64) {
	*b = strconv.AppendFloat(*b, val, 'f', 6, 64)
}

///////////////////////////////////////////////////////////////////////////////

type timeoutWriter struct {
	timeout time.Duration
	conn    net.Conn
	err     error
}

func (w *timeoutWriter) Write(b []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	// CEV: ignore deadline errors as there is nothing we
	// can do about them.
	w.conn.SetWriteDeadline(time.Now().Add(w.timeout))
	var n int
	n, w.err = w.conn.Write(b)
	w.conn.SetWriteDeadline(time.Time{})
	return n, w.err
}
