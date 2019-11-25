package stats

type nullSink struct{}

// NewNullSink returns a Sink that does not have a backing store attached to it.
func NewNullSink() FlushableSink {
	return nullSink{}
}

func (s nullSink) FlushCounter(name string, value uint64) {}

func (s nullSink) FlushGauge(name string, value uint64) {}

func (s nullSink) FlushTimer(name string, value float64) {}

func (s nullSink) Flush() {}
