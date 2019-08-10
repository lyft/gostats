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

	logger "github.com/sirupsen/logrus"
)

// TODO(btc): add constructor that accepts functional options in order to allow
// users to choose the constants that work best for them. (Leave the existing
// c'tor for backwards compatibility)
// e.g. `func NewTCPStatsdSinkWithOptions(opts ...Option) Sink`

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

// NewTCPStatsdSink returns a FlushableSink that is backed by a buffered writer
// and a separate goroutine that flushes those buffers to a statsd connection.
func NewTCPStatsdSink() FlushableSink {
	outc := make(chan *bytes.Buffer, chanSize) // TODO(btc): parameterize
	writer := sinkWriter{
		outc: outc,
	}
	s := &tcpStatsdSink{
		outc: outc,
		// TODO(btc): parameterize size
		bufWriter: bufio.NewWriterSize(&writer, defaultBufferSize),
		// arbitrarily buffered
		doFlush: make(chan struct{}, 8),
	}
	s.flushCond = sync.NewCond(&s.mu)
	go s.run()
	return s
}

type tcpStatsdSink struct {
	conn         net.Conn
	outc         chan *bytes.Buffer
	mu           sync.Mutex
	bufWriter    *bufio.Writer
	flushCond    *sync.Cond
	doFlush      chan struct{}
	droppedBytes uint64
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

	s.doFlush <- struct{}{}
	s.mu.Lock()
	for len(s.outc) != 0 {
		s.flushCond.Wait()
	}
	s.mu.Unlock()
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

// s.mu should be held
func (s *tcpStatsdSink) handleFlushErrorSize(err error, dropped int) {
	d := uint64(dropped)
	if (s.droppedBytes+d)%logOnEveryNDroppedBytes > s.droppedBytes%logOnEveryNDroppedBytes {
		logger.WithField("total_dropped_bytes", s.droppedBytes+d).
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
	conf := GetSettings()
	addr := net.JoinHostPort(conf.StatsdHost, strconv.Itoa(conf.StatsdPort))

	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		if s.conn == nil {
			if err := s.connect(addr); err != nil {
				logger.Warnf("statsd connection error: %s", err)

				// TODO (CEV): don't sleep on the first retry
				time.Sleep(3 * time.Second)
				continue
			}
		}

		select {
		case <-t.C:
			s.flush()
		case <-s.doFlush:
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
			// Signal every blocked Flush() call. We don't handle multiple
			// pending Flush() calls independently as we cannot allow this
			// to block.  This is best effort only.
			//
			s.flushCond.Broadcast()
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
