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
	sink := &tcpStatsdSink{
		outc:     outc,
		flushBuf: bufio.NewWriter(sinkWriter{outc: outc}),
	}
	go sink.run()
	return sink
}

type tcpStatsdSink struct {
	conn         net.Conn
	outc         chan []byte
	droppedBytes uint64
	mu           sync.Mutex
	flushBuf     *bufio.Writer
}

type sinkWriter struct {
	outc chan []byte
}

func (w sinkWriter) Write(p []byte) (int, error) {
	n := len(p)
	pCopy := make([]byte, n)
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
	_, err := fmt.Fprintf(s.flushBuf, f, args...)
	if err != nil {
		s.handleFlushError(err)
	}
	s.mu.Unlock()
}

// s.mu should be held
func (s *tcpStatsdSink) handleFlushError(err error) {
	droppedBytes := uint64(s.flushBuf.Buffered())
	if (s.droppedBytes+droppedBytes)%logOnEveryNDroppedBytes >
		s.droppedBytes%logOnEveryNDroppedBytes {
		logger.WithField("total_dropped_bytes", s.droppedBytes+droppedBytes).
			WithField("dropped_bytes", droppedBytes).
			Error(err)
	}
	s.droppedBytes += droppedBytes
	s.flushBuf.Reset(sinkWriter{outc: s.outc})
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
			if err := s.flushBuf.Flush(); err != nil {
				s.handleFlushError(err)
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
					s.handleFlushError(err)
				} else {
					s.handleFlushError(fmt.Errorf("Short write to statsd. Resetting connection."))
				}
				s.mu.Unlock()
				_ = s.conn.Close() // Ignore close failures
				s.conn = nil
			}
		}
	}
}
