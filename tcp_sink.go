package stats

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
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
	s.flushCond.L = &s.mu
	go s.run()
	return s
}

type tcpStatsdSink struct {
	conn net.Conn
	outc chan *bytes.Buffer

	mu            sync.Mutex
	droppedBytes  uint64
	bufWriter     *bufio.Writer
	flushCond     sync.Cond
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
	err := s.bufWriter.Flush()
	s.mu.Unlock()
	if err != nil {
		s.handleFlushError(err, s.bufWriter.Buffered())
	}
	return err
}

func (s *tcpStatsdSink) flushString(f string, args ...interface{}) {
	s.mu.Lock()
	_, err := fmt.Fprintf(s.bufWriter, f, args...)
	if err != nil {
		s.handleFlushError(err, s.bufWriter.Buffered())
	}
	s.mu.Unlock()
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

func (s *tcpStatsdSink) FlushCounter(name string, value uint64) {
	s.flushString("%s:%d|c\n", name, value)
}

func (s *tcpStatsdSink) FlushGauge(name string, value uint64) {
	s.flushString("%s:%d|g\n", name, value)
}

func (s *tcpStatsdSink) FlushTimer(name string, value float64) {
	s.flushString("%s:%f|ms\n", name, value)
}

func (s *tcpStatsdSink) initConn(settings *Settings) bool {
	if s.conn == nil {
		conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", settings.StatsdHost,
			settings.StatsdPort))
		if err != nil {
			logger.Warnf("statsd connection error: %s", err)
			time.Sleep(3 * time.Second)
			return false
		}
		s.conn = conn
	}
	return true
}

func (s *tcpStatsdSink) run() {
	settings := GetSettings()
	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		if s.conn == nil && !s.initConn(&settings) {
			continue
		}
		select {
		case <-t.C:
			s.flush()
		case buf := <-s.outc: // Receive from the channel and check if the channel has been closed
			lenbuf := buf.Len()
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
