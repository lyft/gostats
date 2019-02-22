package mock

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"
)

type value struct {
	val   uint64
	count int64
}

// A Sink is a mock sink meant for testing that is safe for concurrent use.
type Sink struct {
	counters sync.Map
	timers   sync.Map
	gauges   sync.Map
	mu       sync.RWMutex
}

func NewSink() *Sink {
	return new(Sink)
}

// Flush is a no-op method
func (*Sink) Flush() {}

func (s *Sink) Reset() {
	s.mu.Lock()
	s.counters = sync.Map{}
	s.timers = sync.Map{}
	s.gauges = sync.Map{}
	s.mu.Unlock()
}

func (s *Sink) FlushCounter(name string, val uint64) {
	s.mu.RLock()
	v, ok := s.counters.Load(name)
	if !ok {
		v, _ = s.counters.LoadOrStore(name, new(value))
	}
	s.mu.RUnlock()
	p := v.(*value)
	atomic.AddUint64(&p.val, val)
	atomic.AddInt64(&p.count, 1)
}

func (s *Sink) FlushGauge(name string, val uint64) {
	s.mu.RLock()
	v, ok := s.gauges.Load(name)
	if !ok {
		v, _ = s.gauges.LoadOrStore(name, new(value))
	}
	s.mu.RUnlock()
	p := v.(*value)
	atomic.AddUint64(&p.val, val)
	atomic.AddInt64(&p.count, 1)
}

func atomicAddFloat64(dest *uint64, delta float64) {
	for {
		cur := atomic.LoadUint64(dest)
		curVal := math.Float64frombits(cur)
		nxtVal := curVal + delta
		nxt := math.Float64bits(nxtVal)
		if atomic.CompareAndSwapUint64(dest, cur, nxt) {
			return
		}
	}
}

func (s *Sink) FlushTimer(name string, val float64) {
	s.mu.RLock()
	v, ok := s.timers.Load(name)
	if !ok {
		v, _ = s.timers.LoadOrStore(name, new(value))
	}
	s.mu.RUnlock()
	p := v.(*value)
	atomicAddFloat64(&p.val, val)
	atomic.AddInt64(&p.count, 1)
}

func (s *Sink) LoadCounter(name string) (uint64, bool) {
	s.mu.RLock()
	v, ok := s.counters.Load(name)
	s.mu.RUnlock()
	if ok {
		p := v.(*value)
		return atomic.LoadUint64(&p.val), true
	}
	return 0, false
}

func (s *Sink) LoadGauge(name string) (uint64, bool) {
	s.mu.RLock()
	v, ok := s.gauges.Load(name)
	s.mu.RUnlock()
	if ok {
		p := v.(*value)
		return atomic.LoadUint64(&p.val), true
	}
	return 0, false
}

func (s *Sink) LoadTimer(name string) (float64, bool) {
	s.mu.RLock()
	v, ok := s.timers.Load(name)
	s.mu.RUnlock()
	if ok {
		p := v.(*value)
		bits := atomic.LoadUint64(&p.val)
		return math.Float64frombits(bits), true
	}
	return 0, false
}

// short-hand methods

func (s *Sink) Counter(name string) uint64 {
	v, _ := s.LoadCounter(name)
	return v
}

func (s *Sink) Gauge(name string) uint64 {
	v, _ := s.LoadGauge(name)
	return v
}

func (s *Sink) Timer(name string) float64 {
	v, _ := s.LoadTimer(name)
	return v
}

// these methods are mostly useful for testing

func (s *Sink) CounterCallCount(name string) int64 {
	s.mu.RLock()
	v, ok := s.counters.Load(name)
	s.mu.RUnlock()
	if ok {
		return atomic.LoadInt64(&v.(*value).count)
	}
	return 0
}

func (s *Sink) GaugeCallCount(name string) int64 {
	s.mu.RLock()
	v, ok := s.gauges.Load(name)
	s.mu.RUnlock()
	if ok {
		return atomic.LoadInt64(&v.(*value).count)
	}
	return 0
}

func (s *Sink) TimerCallCount(name string) int64 {
	s.mu.RLock()
	v, ok := s.timers.Load(name)
	s.mu.RUnlock()
	if ok {
		return atomic.LoadInt64(&v.(*value).count)
	}
	return 0
}

// test helpers

// AssertCounterEquals asserts that Counter name is present and has value exp.
func (s *Sink) AssertCounterEquals(tb testing.TB, name string, exp uint64) {
	tb.Helper()
	u, ok := s.LoadCounter(name)
	if !ok {
		tb.Fatalf("gostats/mock: Counter (%q): not found", name)
	}
	if u != exp {
		tb.Fatalf("gostats/mock: Counter (%q): Expected: %d Got: %d", name, exp, u)
	}
}

// AssertGaugeEquals asserts that Gauge name is present and has value exp.
func (s *Sink) AssertGaugeEquals(tb testing.TB, name string, exp uint64) {
	tb.Helper()
	u, ok := s.LoadGauge(name)
	if !ok {
		tb.Fatalf("gostats/mock: Gauge (%q): not found", name)
	}
	if u != exp {
		tb.Fatalf("gostats/mock: Gauge (%q): Expected: %d Got: %d", name, exp, u)
	}
}

// AssertTimerEquals asserts that Timer name is present and has value exp.
func (s *Sink) AssertTimerEquals(tb testing.TB, name string, exp float64) {
	tb.Helper()
	f, ok := s.LoadTimer(name)
	if !ok {
		tb.Fatalf("gostats/mock: Timer (%q): not found", name)
	}
	if f != exp {
		tb.Fatalf("gostats/mock: Timer (%q): Expected: %f Got: %f", name, exp, f)
	}
}

// AssertCounterExists asserts that Counter name exists.
func (s *Sink) AssertCounterExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadCounter(name); !ok {
		tb.Fatalf("gostats/mock: Counter (%q): should exist", name)
	}
}

// AssertGaugeExists asserts that Gauge name exists.
func (s *Sink) AssertGaugeExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadGauge(name); !ok {
		tb.Fatalf("gostats/mock: Gauge (%q): should exist", name)
	}
}

// AssertTimerExists asserts that Timer name exists.
func (s *Sink) AssertTimerExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadTimer(name); !ok {
		tb.Fatalf("gostats/mock: Timer (%q): should exist", name)
	}
}

// AssertCounterNotExists asserts that Counter name does not exist.
func (s *Sink) AssertCounterNotExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadCounter(name); ok {
		tb.Fatalf("gostats/mock: Counter (%q): expected Counter to not exist", name)
	}
}

// AssertGaugeNotExists asserts that Gauge name does not exist.
func (s *Sink) AssertGaugeNotExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadGauge(name); ok {
		tb.Fatalf("gostats/mock: Gauge (%q): expected Gauge to not exist", name)
	}
}

// AssertTimerNotExists asserts that Timer name does not exist.
func (s *Sink) AssertTimerNotExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadTimer(name); ok {
		tb.Fatalf("gostats/mock: Timer (%q): expected Timer to not exist", name)
	}
}

// AssertCounterCallCount asserts that Counter name was called exp times.
func (s *Sink) AssertCounterCallCount(tb testing.TB, name string, exp int) {
	tb.Helper()
	v, ok := s.counters.Load(name)
	if !ok {
		tb.Fatalf("gostats/mock: Counter (%q): not found", name)
	}
	p := v.(*value)
	n := atomic.LoadInt64(&p.count)
	if n != int64(exp) {
		tb.Fatalf("gostats/mock: Counter (%q) Call Count: Expected: %d Got: %d",
			name, exp, n)
	}
}

// AssertGaugeCallCount asserts that Gauge name was called exp times.
func (s *Sink) AssertGaugeCallCount(tb testing.TB, name string, exp int) {
	tb.Helper()
	v, ok := s.gauges.Load(name)
	if !ok {
		tb.Fatalf("gostats/mock: Gauge (%q): not found", name)
	}
	p := v.(*value)
	n := atomic.LoadInt64(&p.count)
	if n != int64(exp) {
		tb.Fatalf("gostats/mock: Gauge (%q) Call Count: Expected: %d Got: %d",
			name, exp, n)
	}
}

// AssertTimerCallCount asserts that Timer name was called exp times.
func (s *Sink) AssertTimerCallCount(tb testing.TB, name string, exp int) {
	tb.Helper()
	v, ok := s.timers.Load(name)
	if !ok {
		tb.Fatalf("gostats/mock: Timer (%q): not found", name)
	}
	p := v.(*value)
	n := atomic.LoadInt64(&p.count)
	if n != int64(exp) {
		tb.Fatalf("gostats/mock: Timer (%q) Call Count: Expected: %d Got: %d",
			name, exp, n)
	}
}
