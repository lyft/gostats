package stats

import logger "github.com/Sirupsen/logrus"

type loggingSink struct {
}

func NewLoggingSink() Sink {
	return &loggingSink{}
}

func (s *loggingSink) FlushCounter(name string, value uint64) {
	logger.Infof("[gostats] flushing counter %s: %d", name, value)
}

func (s *loggingSink) FlushGauge(name string, value uint64) {
	logger.Infof("[gostats] flushing gauge %s: %d", name, value)
}

func (s *loggingSink) FlushTimer(name string, value float64) {
	logger.Infof("[gostats] flushing time %s: %f", name, value)
}
