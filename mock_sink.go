package stats

import "sync"

// MockSink describes an in-memory Sink used for testing.
//
// DEPRECATED: use "github.com/lyft/gostats/mock" instead.
type MockSink struct {
	Counters map[string]uint64
	Timers   map[string]uint64
	Gauges   map[string]uint64

	cLock sync.Mutex
	tLock sync.Mutex
	gLock sync.Mutex
}

// NewMockSink returns a MockSink that flushes stats to in-memory maps. An
// instance of MockSink is not safe for concurrent use.
//
// DEPRECATED: use "github.com/lyft/gostats/mock" instead.
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
	m.cLock.Lock()
	defer m.cLock.Unlock()
	m.Counters[name] += value
}

// FlushGauge satisfies the Sink interface.
func (m *MockSink) FlushGauge(name string, value uint64) {
	m.gLock.Lock()
	defer m.gLock.Unlock()
	m.Gauges[name] = value
}

// FlushTimer satisfies the Sink interface.
func (m *MockSink) FlushTimer(name string, value float64) { //nolint:revive
	m.tLock.Lock()
	defer m.tLock.Unlock()
	m.Timers[name]++
}
