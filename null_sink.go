package stats

type nullSink struct{}

func NewNullSink() Sink {
	return &nullSink{}
}

func (s *nullSink) FlushCounter(name string, value uint64) {}

func (s *nullSink) FlushGauge(name string, value uint64) {}

func (s *nullSink) FlushTimer(name string, value float64) {}
