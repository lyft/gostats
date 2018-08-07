package stats

import logger "github.com/sirupsen/logrus"

type loggingSink struct{}

// NewLoggingSink returns a Sink that flushes stats to os.StdErr.
func NewLoggingSink() FlushableSink {
	return &loggingSink{}
}

func (s *loggingSink) FlushCounter(name string, value uint64) {
	logger.Debugf("[gostats] flushing counter %s: %d", name, value)
}

func (s *loggingSink) FlushGauge(name string, value uint64) {
	logger.Debugf("[gostats] flushing gauge %s: %d", name, value)
}

func (s *loggingSink) FlushTimer(name string, value float64) {
	logger.Debugf("[gostats] flushing time %s: %f", name, value)
}

func (s *loggingSink) Flush() {
	logger.Debugf("[gostats] Flush() called, all stats would be flushed")
}
