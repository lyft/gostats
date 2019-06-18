package stats

type noopSink struct{}

// NewNoopSink returns a FlushableSink that does nothing with stats
//
// Can be used if a non-statsd LoggingSink generates too much log span.
func NewNoopSink() FlushableSink {
	return &loggingSink{}
}

func (s *noopSink) FlushCounter(name string, value uint64) {}

func (s *noopSink) FlushGauge(name string, value uint64) {}

func (s *noopSink) FlushTimer(name string, value float64) {}

func (s *noopSink) Flush() {}

