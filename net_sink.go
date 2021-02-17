package stats

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"net"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	logger "github.com/sirupsen/logrus"
)

const (
	defaultRetryInterval = time.Second * 3
	defaultDialTimeout   = defaultRetryInterval / 2
	defaultWriteTimeout  = time.Second

	flushInterval           = time.Second
	logOnEveryNDroppedBytes = 1 << 15 // Log once per 32kb of dropped stats
	defaultBufferSizeTCP    = 1 << 16

	// 1432 bytes is optimal for regular networks with an MTU of 1500 and
	// is to prevent fragmenting UDP datagrams
	defaultBufferSizeUDP = 1432

	approxMaxMemBytes = 1 << 22
	chanSize          = approxMaxMemBytes / defaultBufferSizeTCP

	STATE_UNLOCKED = 0
	STATE_LOCKED   = 1
)

// An SinkOption configures a Sink.
type SinkOption interface {
	apply(*netSink)
}

// sinkOptionFunc wraps a func so it satisfies the Option interface.
type sinkOptionFunc func(*netSink)

func (f sinkOptionFunc) apply(sink *netSink) {
	f(sink)
}

// WithStatsdHost sets the host of the statsd sink otherwise the host is
// read from the environment variable "STATSD_HOST".
func WithStatsdHost(host string) SinkOption {
	return sinkOptionFunc(func(sink *netSink) {
		sink.conf.StatsdHost = host
	})
}

// WithStatsdProtocol sets the network protocol ("udp" or "tcp") of the statsd
// sink otherwise the protocol is read from the environment variable
// "STATSD_PROTOCOL".
func WithStatsdProtocol(protocol string) SinkOption {
	return sinkOptionFunc(func(sink *netSink) {
		sink.conf.StatsdProtocol = protocol
	})
}

// WithStatsdPort sets the port of the statsd sink otherwise the port is
// read from the environment variable "STATSD_PORT".
func WithStatsdPort(port int) SinkOption {
	return sinkOptionFunc(func(sink *netSink) {
		sink.conf.StatsdPort = port
	})
}

// WithLogger configures the sink to use the provided logger otherwise
// the standard logrus logger is used.
func WithLogger(log *logger.Logger) SinkOption {
	// TODO (CEV): use the zap.Logger
	return sinkOptionFunc(func(sink *netSink) {
		sink.log = log
	})
}

// NewTCPStatsdSink returns a new NetStink. This function name exists for
// backwards compatibility.
func NewTCPStatsdSink(opts ...SinkOption) FlushableSink {
	return NewNetSink(opts...)
}

// NewNetSink returns a FlushableSink that writes to a statsd sink over the
// network. By default settings are taken from the enviornment, but can be
// overridden via SinkOptions.
func NewNetSink(opts ...SinkOption) FlushableSink {
	s := &netSink{
		// arbitrarily buffered
		doFlush: make(chan chan struct{}, 8),

		// CEV: default to the standard logger to match the legacy implementation.
		log: logger.StandardLogger(),

		// TODO (CEV): auto loading from the env is bad and should be removed.
		conf: GetSettings(),
	}
	for _, opt := range opts {
		opt.apply(s)
	}

	// Calculate buffer size based on protocol, for UDP we want to pick a
	// buffer size that will prevent datagram fragmentation.
	var bufSize int
	switch s.conf.StatsdProtocol {
	case "udp", "udp4", "udp6":
		bufSize = defaultBufferSizeUDP
	default:
		bufSize = defaultBufferSizeTCP
	}

	outc := make(chan *bytes.Buffer, approxMaxMemBytes/bufSize)
	writer := &sinkWriter{
		outc: outc,
	}
	s.outc = outc

	s.bufWriter = bufio.NewWriterSize(writer, bufSize)

	go s.run()
	return s
}

type spinLock uint32

func (spin *spinLock) Lock() {
	for !atomic.CompareAndSwapUint32((*uint32)(spin), STATE_UNLOCKED, STATE_LOCKED) {
		runtime.Gosched()
	}
}

func (spin *spinLock) FailableLock() bool {
	return atomic.CompareAndSwapUint32((*uint32)(spin), STATE_UNLOCKED, STATE_LOCKED)
}

func (spin *spinLock) Unlock() {
	atomic.StoreUint32((*uint32)(spin), STATE_UNLOCKED)
}

type netSink struct {
	conn         net.Conn
	outc         chan *bytes.Buffer
	mu           sync.Mutex
	lock         spinLock
	bufWriter    *bufio.Writer
	doFlush      chan chan struct{}
	droppedBytes uint64
	log          *logger.Logger
	conf         Settings
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

func (s *netSink) Flush() {
	if s.flush() != nil {
		return // nothing we can do
	}
	ch := make(chan struct{})
	s.doFlush <- ch
	<-ch
}

func (s *netSink) flush() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	err := s.bufWriter.Flush()
	if err != nil {
		s.handleFlushError(err)
	}
	return err
}

func (s *netSink) drainFlushQueue() {
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
func (s *netSink) handleFlushErrorSize(err error, dropped int) {
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
func (s *netSink) handleFlushError(err error) {
	s.handleFlushErrorSize(err, s.bufWriter.Buffered())
}

func (s *netSink) writeBuffer(b *buffer) {
	if s.lock.FailableLock() {
		defer s.lock.Unlock()
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
	}
}

func (s *netSink) flushUint64(name, suffix string, u uint64) {
	b := pbFree.Get().(*buffer)

	b.WriteString(name)
	b.WriteChar(':')
	b.WriteUnit64(u)
	b.WriteString(suffix)

	s.writeBuffer(b)

	b.Reset()
	pbFree.Put(b)
}

func (s *netSink) flushFloat64(name, suffix string, f float64) {
	b := pbFree.Get().(*buffer)

	b.WriteString(name)
	b.WriteChar(':')
	b.WriteFloat64(f)
	b.WriteString(suffix)

	s.writeBuffer(b)

	b.Reset()
	pbFree.Put(b)
}

func (s *netSink) FlushCounter(name string, value uint64) {
	s.flushUint64(name, "|c\n", value)
}

func (s *netSink) FlushGauge(name string, value uint64) {
	s.flushUint64(name, "|g\n", value)
}

func (s *netSink) FlushTimer(name string, value float64) {
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

func (s *netSink) run() {
	addr := net.JoinHostPort(s.conf.StatsdHost, strconv.Itoa(s.conf.StatsdPort))

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
		case buf := <-s.outc:
			s.writeToConn(buf)
			putBuffer(buf)
		}
	}
}

// writeToConn writes the buffer to the underlying conn.  May only be called
// from run().
func (s *netSink) writeToConn(buf *bytes.Buffer) {

	len := buf.Len()

	// TODO (CEV): parameterize timeout
	s.conn.SetWriteDeadline(time.Now().Add(defaultWriteTimeout))
	_, err := buf.WriteTo(s.conn)
	s.conn.SetWriteDeadline(time.Time{}) // clear

	if err != nil {
		s.lock.Lock()
		s.handleFlushErrorSize(err, len)
		s.lock.Unlock()
		_ = s.conn.Close()
		s.conn = nil // this will break the loop
	}
}

func (s *netSink) connect(address string) error {
	// TODO (CEV): parameterize timeout
	conn, err := net.DialTimeout(s.conf.StatsdProtocol, address, defaultDialTimeout)
	if err == nil {
		s.conn = conn
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
