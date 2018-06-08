package stats

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"

	logger "github.com/sirupsen/logrus"
)

const (
	flushInterval           = time.Second
	logOnEveryNDroppedBytes = 1 << 15 // Log once per 32kb of dropped stats
)

// NewTCPStatsdSink returns a Sink that is backed by a buffered writer and a separate goroutine
// that flushes those buffers to a statsd connection.
func NewTCPStatsdSink() Sink {
	outc := make(chan []byte, 1000)
	writer := sinkWriter{
		outc: outc,
	}
	bufWriter := bufio.NewWriter(writer)
	bufPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, bufWriter.Size())
		},
	}
	s := &tcpStatsdSink{
		outc:      outc,
		bufWriter: bufWriter,
		bufPool:   bufPool,
	}
	writer.bufPool = s.bufPool
	go s.run()
	return s
}

type tcpStatsdSink struct {
	conn         net.Conn
	outc         chan []byte
	droppedBytes uint64
	mu           sync.Mutex
	bufWriter    *bufio.Writer
	bufPool      *sync.Pool
}

type sinkWriter struct {
	bufPool *sync.Pool
	outc    chan<- []byte
}

func (w sinkWriter) Write(p []byte) (int, error) {
	n := len(p)
	pCopy := w.bufPool.Get().([]byte)
	copy(pCopy, p)
	select {
	case w.outc <- pCopy:
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

	s.bufWriter.Reset(sinkWriter{
		bufPool: s.bufPool,
		outc:    s.outc,
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
	t := time.NewTimer(flushInterval)
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
			if !ok {
				logger.Warnf("Closing statsd client")
				s.conn.Close()
				return
			}

			n, err := s.conn.Write(buf)
			if err != nil || n < len(buf) {
				s.mu.Lock()
				if err != nil {
					s.handleFlushError(err, len(buf))
				} else {
					s.handleFlushError(fmt.Errorf("short write to statsd, resetting connection"), len(buf)-n)
				}
				s.mu.Unlock()
				_ = s.conn.Close() // Ignore close failures
				s.conn = nil
			}
			s.bufPool.Put(buf)
		}
	}
}
