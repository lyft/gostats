package stats

// A Sink is used by a Store to flush its data.
type Sink interface {
	FlushCounter(name string, value uint64)
	FlushGauge(name string, value uint64)
	FlushTimer(name string, value float64)
}
