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
		outc:      outc,
		bufWriter: bufio.NewWriterSize(&writer, defaultBufferSize), // TODO(btc): parameterize size
	}
	s.flushCond = sync.NewCond(&s.mu)
	go s.run()
	return s
}

type tcpStatsdSink struct {
	conn net.Conn
	outc chan *bytes.Buffer

	mu            sync.Mutex
	droppedBytes  uint64
	bufWriter     *bufio.Writer
	flushCond     *sync.Cond
	lastFlushTime time.Time
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
	now := time.Now()
	if err := s.flush(); err != nil {
		// Not much we can do here; we don't know how/why we failed.
		return
	}
	s.mu.Lock()
	for now.After(s.lastFlushTime) {
		s.flushCond.Wait()
	}
	s.mu.Unlock()
}

func (s *tcpStatsdSink) flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.bufWriter.Flush()
	if err != nil {
		s.handleFlushError(err, s.bufWriter.Buffered())
		return err
	}
	return nil
}

// s.mu should be held
func (s *tcpStatsdSink) handleFlushError(err error, droppedBytes int) {
	d := uint64(droppedBytes)
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

func (s *tcpStatsdSink) flushUint64(name, suffix string, u uint64) {
	b := pbFree.Get().(*buffer)

	b.WriteString(name)
	b.WriteChar(':')
	b.WriteUnit64(u)
	b.WriteString(suffix)

	s.mu.Lock()
	if _, err := s.bufWriter.Write(*b); err != nil {
		s.handleFlushError(err, s.bufWriter.Buffered())
	}
	s.mu.Unlock()

	b.Reset()
	pbFree.Put(b)
}

func (s *tcpStatsdSink) flushFloat64(name, suffix string, f float64) {
	b := pbFree.Get().(*buffer)

	b.WriteString(name)
	b.WriteChar(':')
	b.WriteFloat64(f)
	b.WriteString(suffix)

	s.mu.Lock()
	if _, err := s.bufWriter.Write(*b); err != nil {
		s.handleFlushError(err, s.bufWriter.Buffered())
	}
	s.mu.Unlock()

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
	settings := GetSettings()
	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		if s.conn == nil {
			conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", settings.StatsdHost,
				settings.StatsdPort))
			if err != nil {
				logger.Warnf("statsd connection error: %s", err)
				time.Sleep(3 * time.Second)
				continue
			}
			s.conn = conn
		}

		select {
		case <-t.C:
			s.flush()
		case buf, ok := <-s.outc: // Receive from the channel and check if the channel has been closed
			if !ok {
				logger.Warnf("Closing statsd client")
				s.conn.Close()
				return
			}
			lenbuf := len(buf.Bytes())
			_, err := buf.WriteTo(s.conn)
			if len(s.outc) == 0 {
				// We've at least tried to write all the data we have. Wake up anyone waiting on flush.
				s.mu.Lock()
				s.lastFlushTime = time.Now()
				s.mu.Unlock()
				s.flushCond.Broadcast()
			}
			if err != nil {
				s.mu.Lock()
				s.handleFlushError(err, lenbuf)
				s.mu.Unlock()
				_ = s.conn.Close() // Ignore close failures
				s.conn = nil
			}
			putBuffer(buf)
		}
	}
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
