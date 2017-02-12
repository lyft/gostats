package stats

// A Store flushes its data to a Sink.
type Sink interface {
	FlushCounter(name string, value uint64)
	FlushGauge(name string, value uint64)
	FlushTimer(name string, value float64)
}
