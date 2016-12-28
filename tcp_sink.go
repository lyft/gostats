package stats

import (
	"bufio"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	logger "github.com/Sirupsen/logrus"
)

const (
	logOnEveryNDropped = 1000
)

func NewTcpStatsdSink() Sink {
	sink := &tcpStatsdSink{
		outc: make(chan string, 1000),
	}
	go sink.run()
	return sink
}

type tcpStatsdSink struct {
	conn            net.Conn
	outc            chan string
	droppedTimers   uint64
	droppedCounters uint64
	droppedGauges   uint64
}

func (s *tcpStatsdSink) FlushCounter(name string, value uint64) {
	select {
	case s.outc <- fmt.Sprintf("%s:%d|c\n", name, value):
	default:
		new := atomic.AddUint64(&s.droppedCounters, 1)
		if new%logOnEveryNDropped == 0 {
			logger.Errorf(
				"statsd channel full, discarding counter flush value, dropped so far %v",
				new)
		}
	}

}

func (s *tcpStatsdSink) FlushGauge(name string, value uint64) {
	select {
	case s.outc <- fmt.Sprintf("%s:%d|g\n", name, value):
	default:
		new := atomic.AddUint64(&s.droppedGauges, 1)
		if new%logOnEveryNDropped == 0 {
			logger.Errorf(
				"statsd channel full, discarding gauge flush value, dropped so far %v",
				new)
		}
	}
}

func (s *tcpStatsdSink) FlushTimer(name string, value float64) {
	select {
	case s.outc <- fmt.Sprintf("%s:%f|ms\n", name, value):
	default:
		new := atomic.AddUint64(&s.droppedTimers, 1)
		if new%logOnEveryNDropped == 0 {
			logger.Errorf(
				"statsd channel full, discarding timer flush value, dropped so far %v",
				new)
		}
	}
}

func (w *tcpStatsdSink) run() {
	settings := GetSettings()
	var writer *bufio.Writer
	var err error
	for {
		if w.conn == nil {
			w.conn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", settings.StatsdHost,
				settings.StatsdPort))
			if err != nil {
				logger.Warnf("statsd connection error: %s", err)
				time.Sleep(3 * time.Second)
				continue
			}
			writer = bufio.NewWriter(w.conn)
		}

		// Receive from the channel and check if the channel has been closed
		metric, ok := <-w.outc
		if !ok {
			logger.Warnf("Closing statsd client")
			w.conn.Close()
			return
		}

		writer.WriteString(metric)
		err = writer.Flush()

		if err != nil {
			logger.Warnf("Writing to statsd failed: %s", err)
			_ = w.conn.Close() // Ignore close failures
			w.conn = nil
			writer = nil
		}
	}
}
