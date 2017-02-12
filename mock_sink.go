package stats

type mockSink struct {
	Counters map[string]uint64
	Timers   map[string]uint64
	Gauges   map[string]uint64
}

// Returns a Mock Sink that flushes stats to in memory maps.
func NewMockSink() (m *mockSink) {
	m = &mockSink{
		Counters: make(map[string]uint64),
		Timers:   make(map[string]uint64),
		Gauges:   make(map[string]uint64),
	}

	return
}

func (m *mockSink) FlushCounter(name string, value uint64) {
	m.Counters[name] += value
}

func (m *mockSink) FlushGauge(name string, value uint64) {
	m.Gauges[name] = value
}

func (m *mockSink) FlushTimer(name string, value float64) {
	m.Timers[name]++
}
