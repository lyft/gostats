package stats

// MockSink describes an in-memory Sink used for testing.
type MockSink struct {
	Counters map[string]uint64
	Timers   map[string]uint64
	Gauges   map[string]uint64
}

// NewMockSink returns a MockSink that flushes stats to in-memory maps. An
// instance of MockSink is not safe for concurrent use.
func NewMockSink() (m *MockSink) {
	m = &MockSink{
		Counters: make(map[string]uint64),
		Timers:   make(map[string]uint64),
		Gauges:   make(map[string]uint64),
	}

	return
}

// FlushCounter satisfies the Sink interface.
func (m *MockSink) FlushCounter(name string, value uint64) {
	m.Counters[name] += value
}

// FlushGauge satisfies the Sink interface.
func (m *MockSink) FlushGauge(name string, value uint64) {
	m.Gauges[name] = value
}

// FlushTimer satisfies the Sink interface.
func (m *MockSink) FlushTimer(name string, value float64) {
	m.Timers[name]++
}
