package stats

// A Sink is used by a Store to flush its data.
// These functions may buffer the given data.
type Sink interface {
	FlushCounter(name string, value uint64)
	FlushGauge(name string, value uint64)
	FlushTimer(name string, value float64)
}

// FlushableSink is an extension of Sink that provides a Flush() function that
// will flush any buffered stats to the underlying store.
type FlushableSink interface {
	Sink
	Flush()
}
