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
	return newTCPStatsdSink(&netDialer{}, nil)
}

func newTCPStatsdSink(d dialer, writtenChan chan struct{}) *tcpStatsdSink {
	outc := make(chan *bytes.Buffer, chanSize) // TODO(btc): parameterize
	writer := sinkWriter{
		outc: outc,
	}
	bufWriter := bufio.NewWriterSize(&writer, defaultBufferSize) // TODO(btc): parameterize size
	pool := newBufferPool(defaultBufferSize)
	s := &tcpStatsdSink{
		dialer:      d,
		outc:        outc,
		bufWriter:   bufWriter,
		pool:        pool,
		writtenChan: writtenChan,
	}
	writer.pool = s.pool
	go s.run()
	return s
}

type conn interface {
	Write(b []byte) (n int, err error)
	Close() error
}

type dialer interface {
	Dial(network, address string) (conn, error)
}

type netDialer struct{}

func (d *netDialer) Dial(network, address string) (conn, error) {
	return net.Dial(network, address)
}

type tcpStatsdSink struct {
	c      conn
	dialer dialer
	outc   chan *bytes.Buffer
	pool   *bufferpool

	mu           sync.Mutex
	droppedBytes uint64
	bufWriter    *bufio.Writer
	writtenChan  chan struct{}
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
		return n, nil
	default:
		return 0, fmt.Errorf("statsd channel full, dropping stats buffer with %d bytes", n)
	}
}

func (s *tcpStatsdSink) flush(f string, args ...interface{}) {
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
		pool: s.pool,
		outc: s.outc,
	})
}

func (s *tcpStatsdSink) FlushCounter(name string, value uint64) {
	s.flush("%s:%d|c\n", name, value)
}

func (s *tcpStatsdSink) FlushGauge(name string, value uint64) {
	s.flush("%s:%d|g\n", name, value)
}

func (s *tcpStatsdSink) FlushTimer(name string, value float64) {
	s.flush("%s:%f|ms\n", name, value)
}

func (s *tcpStatsdSink) run() {
	settings := GetSettings()
	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		if s.c == nil {
			c, err := s.dialer.Dial("tcp", fmt.Sprintf("%s:%d", settings.StatsdHost,
				settings.StatsdPort))
			if err != nil {
				logger.Warnf("statsd connection error: %s", err)
				time.Sleep(3 * time.Second)
				continue
			}
			s.c = c
		}

		select {
		case <-t.C:
			t.Reset(time.Second)
			s.mu.Lock()
			if err := s.bufWriter.Flush(); err != nil {
				s.handleFlushError(err, s.bufWriter.Buffered())
			}
			s.mu.Unlock()
		case buf, ok := <-s.outc: // Receive from the channel and check if the channel has been closed
			if !ok {
				logger.Warnf("Closing statsd client")
				s.c.Close()
				return
			}

			lenbuf := len(buf.Bytes())
			n, err := s.c.Write(buf.Bytes())
			if s.writtenChan != nil {
				s.writtenChan <- struct{}{}
			}
			if err != nil || n < lenbuf {
				s.mu.Lock()
				if err != nil {
					s.handleFlushError(err, lenbuf)
				} else {
					fmt.Println(lenbuf)
					s.handleFlushError(fmt.Errorf("short write to statsd, resetting connection"), lenbuf-n)
				}
				s.mu.Unlock()
				_ = s.c.Close() // Ignore close failures
				s.c = nil
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
