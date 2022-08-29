package mock

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/lyft/gostats/internal/tags"
)

type entry struct {
	val   uint64
	count int64
}

type sink struct {
	counters sync.Map
	timers   sync.Map
	gauges   sync.Map
}

// A Sink is a mock sink meant for testing that is safe for concurrent use.
type Sink struct {
	store atomic.Value
	once  sync.Once
}

func (s *Sink) sink() *sink {
	s.once.Do(func() { s.store.Store(new(sink)) })
	return s.store.Load().(*sink)
}

func (s *Sink) counters() *sync.Map { return &s.sink().counters }
func (s *Sink) timers() *sync.Map   { return &s.sink().timers }
func (s *Sink) gauges() *sync.Map   { return &s.sink().gauges }

// NewSink returns a new Sink which implements the stats.Sink interface and is
// suitable for testing.
func NewSink() *Sink {
	s := &Sink{}
	s.sink() // lazy init
	return s
}

// Flush is a no-op method
func (*Sink) Flush() {}

// Reset resets the Sink's counters, timers and gauges to zero.
func (s *Sink) Reset() {
	s.store.Store(new(sink))
}

// FlushCounter implements the stats.Sink.FlushCounter method and adds val to
// stat name.
func (s *Sink) FlushCounter(name string, val uint64) {
	counters := s.counters()
	v, ok := counters.Load(name)
	if !ok {
		v, _ = counters.LoadOrStore(name, new(entry))
	}
	p := v.(*entry)
	atomic.AddUint64(&p.val, val)
	atomic.AddInt64(&p.count, 1)
}

// FlushGauge implements the stats.Sink.FlushGauge method and adds val to
// stat name.
func (s *Sink) FlushGauge(name string, val uint64) {
	gauges := s.gauges()
	v, ok := gauges.Load(name)
	if !ok {
		v, _ = gauges.LoadOrStore(name, new(entry))
	}
	p := v.(*entry)
	atomic.StoreUint64(&p.val, val)
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

// FlushTimer implements the stats.Sink.FlushTimer method and adds val to
// stat name.
func (s *Sink) FlushTimer(name string, val float64) {
	timers := s.timers()
	v, ok := timers.Load(name)
	if !ok {
		v, _ = timers.LoadOrStore(name, new(entry))
	}
	p := v.(*entry)
	atomicAddFloat64(&p.val, val)
	atomic.AddInt64(&p.count, 1)
}

// LoadCounter returns the value for stat name and if it was found.
func (s *Sink) LoadCounter(name string) (uint64, bool) {
	v, ok := s.counters().Load(name)
	if ok {
		p := v.(*entry)
		return atomic.LoadUint64(&p.val), true
	}
	return 0, false
}

// LoadGauge returns the value for stat name and if it was found.
func (s *Sink) LoadGauge(name string) (uint64, bool) {
	v, ok := s.gauges().Load(name)
	if ok {
		p := v.(*entry)
		return atomic.LoadUint64(&p.val), true
	}
	return 0, false
}

// LoadTimer returns the value for stat name and if it was found.
func (s *Sink) LoadTimer(name string) (float64, bool) {
	v, ok := s.timers().Load(name)
	if ok {
		p := v.(*entry)
		bits := atomic.LoadUint64(&p.val)
		return math.Float64frombits(bits), true
	}
	return 0, false
}

// ListCounters returns a list of existing counter names.
func (s *Sink) ListCounters() []string {
	return keys(s.counters())
}

// ListGauges returns a list of existing gauge names.
func (s *Sink) ListGauges() []string {
	return keys(s.gauges())
}

// ListTimers returns a list of existing timer names.
func (s *Sink) ListTimers() []string {
	return keys(s.timers())
}

// Note, this may return an incoherent snapshot if contents is being concurrently modified
func keys(m *sync.Map) (a []string) {
	m.Range(func(key interface{}, _ interface{}) bool {
		a = append(a, key.(string))
		return true
	})
	return a
}

// Counters returns all the counters currently stored by the sink.
func (s *Sink) Counters() map[string]uint64 {
	m := make(map[string]uint64)
	s.counters().Range(func(k, v interface{}) bool {
		p := v.(*entry)
		m[k.(string)] = atomic.LoadUint64(&p.val)
		return true
	})
	return m
}

// Gauges returns all the gauges currently stored by the sink.
func (s *Sink) Gauges() map[string]uint64 {
	m := make(map[string]uint64)
	s.gauges().Range(func(k, v interface{}) bool {
		p := v.(*entry)
		m[k.(string)] = atomic.LoadUint64(&p.val)
		return true
	})
	return m
}

// Timers returns all the timers currently stored by the sink.
func (s *Sink) Timers() map[string]float64 {
	m := make(map[string]float64)
	s.timers().Range(func(k, v interface{}) bool {
		p := v.(*entry)
		bits := atomic.LoadUint64(&p.val)
		m[k.(string)] = math.Float64frombits(bits)
		return true
	})
	return m
}

// short-hand methods

// Counter is shorthand for LoadCounter, zero is returned if the stat is not found.
func (s *Sink) Counter(name string) uint64 {
	v, _ := s.LoadCounter(name)
	return v
}

// Gauge is shorthand for LoadGauge, zero is returned if the stat is not found.
func (s *Sink) Gauge(name string) uint64 {
	v, _ := s.LoadGauge(name)
	return v
}

// Timer is shorthand for LoadTimer, zero is returned if the stat is not found.
func (s *Sink) Timer(name string) float64 {
	v, _ := s.LoadTimer(name)
	return v
}

// these methods are mostly useful for testing

// CounterCallCount returns the number of times stat name has been called/updated.
func (s *Sink) CounterCallCount(name string) int64 {
	v, ok := s.counters().Load(name)
	if ok {
		return atomic.LoadInt64(&v.(*entry).count)
	}
	return 0
}

// GaugeCallCount returns the number of times stat name has been called/updated.
func (s *Sink) GaugeCallCount(name string) int64 {
	v, ok := s.gauges().Load(name)
	if ok {
		return atomic.LoadInt64(&v.(*entry).count)
	}
	return 0
}

// TimerCallCount returns the number of times stat name has been called/updated.
func (s *Sink) TimerCallCount(name string) int64 {
	v, ok := s.timers().Load(name)
	if ok {
		return atomic.LoadInt64(&v.(*entry).count)
	}
	return 0
}

// test helpers

// AssertCounterEquals asserts that Counter name is present and has value exp.
func (s *Sink) AssertCounterEquals(tb testing.TB, name string, exp uint64) {
	tb.Helper()
	u, ok := s.LoadCounter(name)
	if !ok {
		tb.Errorf("gostats/mock: Counter (%q): not found in: %q", name, s.ListCounters())
		return
	}
	if u != exp {
		tb.Errorf("gostats/mock: Counter (%q): Expected: %d Got: %d", name, exp, u)
	}
}

// AssertGaugeEquals asserts that Gauge name is present and has value exp.
func (s *Sink) AssertGaugeEquals(tb testing.TB, name string, exp uint64) {
	tb.Helper()
	u, ok := s.LoadGauge(name)
	if !ok {
		tb.Errorf("gostats/mock: Gauge (%q): not found in: %q", name, s.ListGauges())
		return
	}
	if u != exp {
		tb.Errorf("gostats/mock: Gauge (%q): Expected: %d Got: %d", name, exp, u)
	}
}

// AssertTimerEquals asserts that Timer name is present and has value exp.
func (s *Sink) AssertTimerEquals(tb testing.TB, name string, exp float64) {
	tb.Helper()
	f, ok := s.LoadTimer(name)
	if !ok {
		tb.Errorf("gostats/mock: Timer (%q): not found in: %q", name, s.ListTimers())
		return
	}
	if f != exp {
		tb.Errorf("gostats/mock: Timer (%q): Expected: %f Got: %f", name, exp, f)
	}
}

// AssertCounterExists asserts that Counter name exists.
func (s *Sink) AssertCounterExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadCounter(name); !ok {
		tb.Errorf("gostats/mock: Counter (%q): not found in: %q", name, s.ListCounters())
	}
}

// AssertGaugeExists asserts that Gauge name exists.
func (s *Sink) AssertGaugeExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadGauge(name); !ok {
		tb.Errorf("gostats/mock: Gauge (%q): not found in: %q", name, s.ListGauges())
	}
}

// AssertTimerExists asserts that Timer name exists.
func (s *Sink) AssertTimerExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadTimer(name); !ok {
		tb.Errorf("gostats/mock: Timer (%q): not found in: %q", name, s.ListTimers())
	}
}

// AssertCounterNotExists asserts that Counter name does not exist.
func (s *Sink) AssertCounterNotExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadCounter(name); ok {
		tb.Errorf("gostats/mock: Counter (%q): expected Counter to not exist", name)
	}
}

// AssertGaugeNotExists asserts that Gauge name does not exist.
func (s *Sink) AssertGaugeNotExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadGauge(name); ok {
		tb.Errorf("gostats/mock: Gauge (%q): expected Gauge to not exist", name)
	}
}

// AssertTimerNotExists asserts that Timer name does not exist.
func (s *Sink) AssertTimerNotExists(tb testing.TB, name string) {
	tb.Helper()
	if _, ok := s.LoadTimer(name); ok {
		tb.Errorf("gostats/mock: Timer (%q): expected Timer to not exist", name)
	}
}

// AssertCounterCallCount asserts that Counter name was called exp times.
func (s *Sink) AssertCounterCallCount(tb testing.TB, name string, exp int) {
	tb.Helper()
	v, ok := s.counters().Load(name)
	if !ok {
		tb.Errorf("gostats/mock: Counter (%q): not found in: %q", name, s.ListCounters())
		return
	}
	p := v.(*entry)
	n := atomic.LoadInt64(&p.count)
	if n != int64(exp) {
		tb.Errorf("gostats/mock: Counter (%q) Call Count: Expected: %d Got: %d",
			name, exp, n)
	}
}

// AssertGaugeCallCount asserts that Gauge name was called exp times.
func (s *Sink) AssertGaugeCallCount(tb testing.TB, name string, exp int) {
	tb.Helper()
	v, ok := s.gauges().Load(name)
	if !ok {
		tb.Errorf("gostats/mock: Gauge (%q): not found in: %q", name, s.ListGauges())
		return
	}
	p := v.(*entry)
	n := atomic.LoadInt64(&p.count)
	if n != int64(exp) {
		tb.Errorf("gostats/mock: Gauge (%q) Call Count: Expected: %d Got: %d",
			name, exp, n)
	}
}

// AssertTimerCallCount asserts that Timer name was called exp times.
func (s *Sink) AssertTimerCallCount(tb testing.TB, name string, exp int) {
	tb.Helper()
	v, ok := s.timers().Load(name)
	if !ok {
		tb.Errorf("gostats/mock: Timer (%q): not found in: %q", name, s.ListTimers())
		return
	}
	p := v.(*entry)
	n := atomic.LoadInt64(&p.count)
	if n != int64(exp) {
		tb.Errorf("gostats/mock: Timer (%q) Call Count: Expected: %d Got: %d",
			name, exp, n)
	}
}

var (
	_ testing.TB = (*fatalTest)(nil)
	_ testing.TB = (*fatalBench)(nil)
)

type fatalTest testing.T

func (t *fatalTest) Errorf(format string, args ...interface{}) {
	t.Fatalf(format, args...)
}

type fatalBench testing.B

func (t *fatalBench) Errorf(format string, args ...interface{}) {
	t.Fatalf(format, args...)
}

// Fatal is a wrapper around *testing.T and *testing.B that causes Sink Assert*
// methods to immediately fail a test and stop execution. Otherwise, the Assert
// methods call tb.Errorf(), which marks the test as failed, but allows
// execution to continue.
//
// Examples of Fatal() can be found in the sink test code.
//
//	var sink Sink
//	var t *testing.T
//	sink.AssertCounterEquals(Must(t), "name", 1)
func Fatal(tb testing.TB) testing.TB {
	switch t := tb.(type) {
	case *testing.T:
		return (*fatalTest)(t)
	case *testing.B:
		return (*fatalBench)(t)
	default:
		panic(fmt.Sprintf("invalid type for testing.TB: %T", tb))
	}
}

// ParseTags extracts the name and tags from a statsd stat.
//
// Example of parsing tags and the stat name from a statsd stat:
//
//	expected := map[string]string{
//		"_f":   "i",
//		"tag1": "value1",
//	}
//	name, tags := mock.ParseTags("prefix.c.___f=i.__tag1=value1")
//	if name != "panic.c" {
//		panic(fmt.Sprintf("Name: got: %q want: %q", name, "panic.c"))
//	}
//	if !reflect.DeepEqual(tags, expected) {
//		panic(fmt.Sprintf("Tags: got: %q want: %q", tags, expected))
//	}
func ParseTags(stat string) (string, map[string]string) {
	return tags.ParseTags(stat)
}

// SerializeTags serializes name and tags into a statsd stat.
//
//	tags := map[string]string{
//		"key_1": "val_1"
//		"key_2": "val_2"
//	}
//	s.AssertCounterExists(tb, SerializeTags("name", tags))
func SerializeTags(name string, tagsm map[string]string) string {
	return tags.SerializeTags(name, tagsm)
}
