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

// NewTCPStatsdSink returns a Sink that is backed by a buffered writer and a separate goroutine
// that flushes those buffers to a statsd connection.
func NewTCPStatsdSink() Sink {
	outc := make(chan *bytes.Buffer, chanSize) // TODO(btc): parameterize
	flushedc := make(chan bool)
	writer := sinkWriter{
		outc: outc,
	}
	bufWriter := bufio.NewWriterSize(&writer, defaultBufferSize) // TODO(btc): parameterize size
	pool := newBufferPool(defaultBufferSize)
	s := &tcpStatsdSink{
		outc:      outc,
		flushedc:  flushedc,
		bufWriter: bufWriter,
		pool:      pool,
	}
	writer.pool = s.pool
	go s.run()
	return s
}

type tcpStatsdSink struct {
	conn     net.Conn
	outc     chan *bytes.Buffer
	flushedc chan bool
	pool     *bufferpool

	mu           sync.Mutex
	droppedBytes uint64
	bufWriter    *bufio.Writer
}

type sinkWriter struct {
	pool *bufferpool
	outc chan<- *bytes.Buffer
}

func (w *sinkWriter) Write(p []byte) (int, error) {
	n := len(p)
	dest := w.pool.Get()
	dest.Write(p)
	select {
	case w.outc <- dest:
		logger.Debug("wrote to outc chan")
		return n, nil
	default:
		return 0, fmt.Errorf("statsd channel full, dropping stats buffer with %d bytes", n)
	}
}

func (s *tcpStatsdSink) flush(f string, args ...interface{}) {
	s.mu.Lock()
	_, err := fmt.Fprintf(s.bufWriter, f, args...)
	logger.Debug("fmt.Fprintf to bufWriter")
	if err != nil {
		s.handleFlushError(err, s.bufWriter.Buffered())
	}
	s.mu.Unlock()
	//<-s.flushedc
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
		pool: s.pool,
		outc: s.outc,
	})
}

func (s *tcpStatsdSink) FlushCounter(name string, value uint64) {
	logger.Debug("FlushCounter called")
	s.flush("%s:%d|c\n", name, value)
	logger.Debug("FlushCounter returning")
}

func (s *tcpStatsdSink) FlushGauge(name string, value uint64) {
	logger.Debug("FlushGauge called")
	s.flush("%s:%d|g\n", name, value)
	logger.Debug("FlushGauge returning")
}

func (s *tcpStatsdSink) FlushTimer(name string, value float64) {
	logger.Debug("FlushTimer called")
	s.flush("%s:%f|ms\n", name, value)
	logger.Debug("FlushTimer returning")
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
			s.mu.Lock()
			if err := s.bufWriter.Flush(); err != nil {
				s.handleFlushError(err, s.bufWriter.Buffered())
			}
			s.mu.Unlock()
		case buf, ok := <-s.outc: // Receive from the channel and check if the channel has been closed
			logger.Debug("received from outc chan")
			if !ok {
				logger.Warnf("Closing statsd client")
				s.conn.Close()
				return
			}

			lenbuf := len(buf.Bytes())
			n, err := s.conn.Write(buf.Bytes())
			//s.flushedc <- true
			logger.Debug("wrote to conn")
			if err != nil || n < lenbuf {
				s.mu.Lock()
				if err != nil {
					s.handleFlushError(err, lenbuf)
				} else {
					s.handleFlushError(fmt.Errorf("short write to statsd, resetting connection"), lenbuf-n)
				}
				s.mu.Unlock()
				_ = s.conn.Close() // Ignore close failures
				s.conn = nil
			}
			s.pool.Put(buf)
		}
	}
}

type bufferpool struct {
	pool sync.Pool
}

func newBufferPool(defaultSizeBytes int) *bufferpool {
	p := new(bufferpool)
	p.pool.New = func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, defaultSizeBytes))
	}
	return p
}

func (p *bufferpool) Put(b *bytes.Buffer) {
	b.Reset()
	p.pool.Put(b)
}

func (p *bufferpool) Get() *bytes.Buffer {
	return p.pool.Get().(*bytes.Buffer)
}
