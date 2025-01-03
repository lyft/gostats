package stats

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

// Logger is used to log errors and other important operational
// information while using gostats.
//
// For convenience of transitioning from logrus to zap, this interface
// conforms BOTH to logrus.Logger as well as the Zap's Sugared logger.
type Logger interface {
	Errorf(msg string, args ...interface{})
	Warnf(msg string, args ...interface{})
}

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
// the built-in zap-like logger is used.
func WithLogger(log Logger) SinkOption {
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
// network. By default settings are taken from the environment, but can be
// overridden via SinkOptions.
func NewNetSink(opts ...SinkOption) FlushableSink {
	s := &netSink{
		// arbitrarily buffered
		doFlush: make(chan chan struct{}, 8),

		// default logging sink mimics the previously-used logrus
		// logger by logging to stderr
		log: &loggingSink{writer: os.Stderr, now: time.Now},

		// TODO (CEV): auto loading from the env is bad and should be removed.
		conf: GetSettings(),
	}
	for _, opt := range opts {
		opt.apply(s)
	}

	// Calculate buffer size based on protocol, for UDP we want to pick a
	// buffer size that will prevent datagram fragmentation of a single stat.
	var bufSize int
	switch s.conf.StatsdProtocol {
	case "udp", "udp4", "udp6":
		bufSize = defaultBufferSizeUDP
	default:
		bufSize = defaultBufferSizeTCP
	}

	s.outc = make(chan *bytes.Buffer, approxMaxMemBytes/bufSize) // this will reduce the maximum number stats we send when draining/sending pending buffers (doFlush) but won't limit batched sends in the same way
	s.retryc = make(chan *bytes.Buffer, 1)                       // It should be okay to limit this given once we write to the retry channel we break the connection, and in subsequent processing we preferentially process from this over outc.

	writer := &sinkWriter{outc: s.outc}
	s.bufWriter = bufio.NewWriterSize(writer, bufSize)

	go s.run()
	return s
}

type netSink struct {
	conn         net.Conn
	outc         chan *bytes.Buffer
	retryc       chan *bytes.Buffer
	mu           sync.Mutex
	bufWriter    *bufio.Writer
	doFlush      chan chan struct{}
	droppedBytes uint64
	log          Logger
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

// flush the entire buffer to outc and drain it, either sending directly or batching
func (s *netSink) Flush() {
	if s.flush() != nil {
		return // nothing we can do
	}
	ch := make(chan struct{})
	s.doFlush <- ch
	<-ch
}

func (s *netSink) flush() error {
	s.mu.Lock()
	err := s.bufWriter.Flush()
	if err != nil {
		s.handleFlushError(err)
	}
	s.mu.Unlock()
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
		s.log.Errorf("dropped %d bytes: %s", s.droppedBytes+d, err)
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

	isBatchEnabled := s.conf.BatchEnabled

	batchSize := s.conf.BatchSize
	batch := make([]bytes.Buffer, 0, batchSize+cap(s.outc)) // overallocate to consider draining outstanding outc data. despite the expanded allocation, batchSize is still used to determine if we send the batched stats

	batchTimeout := time.Duration(s.conf.BatchSendIntervalS) * time.Second
	batchInterval := time.After(batchTimeout)

	sendBatch := false

	t := time.NewTicker(flushInterval)
	defer t.Stop()

	// outc send loop (if batching is enabled, send every batchTimeout, if indicated by doFlush, or if the batch is full)
	// auto-flushes any/all buffered stats (on specified ticker) to outc
	for {
		// writeToConn will set s.conn to nil on error, try to reconnect
		if s.conn == nil {
			// connect to statsd server
			// the connection will be persisted unless an error occurs. thereby with batching is used the entire batch will be streamed under 1 connection
			if err := s.connect(addr); err != nil {
				s.log.Warnf("connection error: %s", err)

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

		// Drain all retries first and continue loop if there are multiple things to retry
		select {
		case buf := <-s.retryc:
			if err := s.writeToConn(buf); err != nil {
				s.mu.Lock()
				s.handleFlushErrorSize(err, buf.Len())
				s.mu.Unlock()
			}
			putBuffer(buf)
			continue // if error we may need to reconnect so go back to the top of the next iteration
		default:
			// Drop through in case retryc has nothing.
		}

		// send batched outc data anytime indicated or the batch is full
		if sendBatch || len(batch) >= batchSize {
			var err error
			batch, err = s.sendBatch(batch)
			if err != nil {
				sendBatch = true // indicate to continue sending out the batch in the next iterration if any erros occur (needed if entering from len(batch) >= batchSize)
				continue         // cut the iteration to process retry (s.conn will be nil anyway)
			}
			sendBatch = false
			batchInterval = time.After(batchTimeout)
		}

		select {
		// flush buffer to outc
		// from a higher level, stats are written to s.bufWriter.buf at GOSTATS_FLUSH_INTERVAL_SECONDS (or adhoc for Timers). this flushes them to outc every t.C tick
		case <-t.C:
			s.flush() // if outc is or becomes full the sinkWriter will error and we will gracefully drop the remainding buffered stats

			select {
			case <-batchInterval:
				if len(batch) > 0 {
					sendBatch = true
				} else {
					batchInterval = time.After(batchTimeout)
				}

			default:
			}
		// drain all outc data. either batch or send immediatedly
		case done := <-s.doFlush:
			if isBatchEnabled {
				n := len(s.outc)
				for i := 0; i < n && s.conn != nil; i++ {
					buf := <-s.outc
					batch = append(batch, *buf)
				}
				sendBatch = true // indicate batched outc data to be sent in the next iteration
			} else {
				n := len(s.outc) // Only flush pending buffers, this prevents an issue where continuous writes prevent the flush loop from exiting.
				for i := 0; i < n && s.conn != nil; i++ {
					buf := <-s.outc
					if err := s.send(buf); err == nil {
						putBuffer(buf)
					}
					// if an error occurs we send the metric to the retry channel and make s.conn nil, breaking this loop and preventing further batch processing in this stack
				}
			}

			close(done)
		// drain one stat from outc if avaible. either batch or send immediatedly
		case buf := <-s.outc:
			if isBatchEnabled {
				batch = append(batch, *buf)
				continue
			}

			if err := s.send(buf); err == nil {
				putBuffer(buf)
			}
		}
	}
}

func (s *netSink) send(buf *bytes.Buffer) error {
	if err := s.writeToConn(buf); err != nil {
		s.retryc <- buf
		return err
	}
	return nil
}

func (s *netSink) sendBatch(batch []bytes.Buffer) ([]bytes.Buffer, error) {
	n := len(batch)

	var i int
	for i = 0; i < n && s.conn != nil; i++ {
		buf := batch[i]
		if err := s.send(&buf); err == nil {
			putBuffer(&buf)
		}
		// if an error occurs we send the metric to the retry channel and make s.conn nil, breaking this loop and preventing further batch processing in this stack
	}

	var err error
	if i != n {
		err = fmt.Errorf("batch send failure, only sent %d of %d", i, n)
	}

	return batch[i:n:n], err
}

// writeToConn writes the buffer to the underlying conn.  May only be called
// from run().
func (s *netSink) writeToConn(buf *bytes.Buffer) error {
	// TODO (CEV): parameterize timeout
	s.conn.SetWriteDeadline(time.Now().Add(defaultWriteTimeout))
	_, err := buf.WriteTo(s.conn)
	s.conn.SetWriteDeadline(time.Time{}) // clear

	if err != nil {
		_ = s.conn.Close()
		s.conn = nil // this will break the loop
	}
	return err
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
//	func (b *buffer) WriteByte(c byte) error { ... }
func (b *buffer) WriteChar(c byte) {
	*b = append(*b, c)
}

func (b *buffer) WriteUnit64(val uint64) {
	*b = strconv.AppendUint(*b, val, 10)
}

func (b *buffer) WriteFloat64(val float64) {
	*b = strconv.AppendFloat(*b, val, 'f', 6, 64)
}
