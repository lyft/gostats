package stats

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	tagspkg "github.com/lyft/gostats/internal/tags"
)

// A Store holds statistics.
// There are two options when creating a new store:
//
//	create a store backed by a tcp_sink to statsd
//	s := stats.NewDefaultStore()
//	create a store with a user provided Sink
//	s := stats.NewStore(sink, true)
//
// Currently that only backing store supported is statsd via a TCP sink, https://github.com/lyft/gostats/blob/master/tcp_sink.go.
// However, implementing other Sinks (https://github.com/lyft/gostats/blob/master/sink.go) should be simple.
//
// A store holds Counters, Gauges, and Timers. You can add unscoped Counters, Gauges, and Timers to the store
// with:
//
//	s := stats.NewDefaultStore()
//	c := s.New[Counter|Gauge|Timer]("name")
type Store interface {
	// Flush Counters and Gauges to the Sink attached to the Store.
	// To flush the store at a regular interval call the
	//  Start(*time.Ticker)
	// method on it.
	//
	// The store will flush either at the regular interval, or whenever
	//  Flush()
	// is called. Whenever the store is flushed,
	// the store will call
	//  GenerateStats()
	// on all of its stat generators,
	// and flush all the Counters and Gauges registered with it.
	Flush()

	// Start a timer for periodic stat flushes. This is a blocking
	// call and should be called in a goroutine.
	Start(*time.Ticker)

	// StartContext starts a timer for periodic stat flushes. This is
	// a blocking call and should be called in a goroutine.
	//
	// If the passed-in context is cancelled, then this call
	// exits. Flush will be called on exit.
	StartContext(context.Context, *time.Ticker)

	// Add a StatGenerator to the Store that programatically generates stats.
	AddStatGenerator(StatGenerator)
	Scope
}

// A Scope namespaces Statistics.
//
//	store := stats.NewDefaultStore()
//	scope := stats.Scope("service")
//	// the following counter will be emitted at the stats tree rooted at `service`.
//	c := scope.NewCounter("success")
//
// Additionally you can create subscopes:
//
//	store := stats.NewDefaultStore()
//	scope := stats.Scope("service")
//	networkScope := scope.Scope("network")
//	// the following counter will be emitted at the stats tree rooted at service.network.
//	c := networkScope.NewCounter("requests")
type Scope interface {
	// Scope creates a subscope.
	Scope(name string) Scope

	// ScopeWithTags creates a subscope with Tags to a store or scope. All child scopes and metrics
	// will inherit these tags by default.
	ScopeWithTags(name string, tags map[string]string) Scope

	// Store returns the Scope's backing Store.
	Store() Store

	// NewCounter adds a Counter to a store, or a scope.
	NewCounter(name string) Counter

	// NewCounterWithTags adds a Counter with Tags to a store, or a scope.
	NewCounterWithTags(name string, tags map[string]string) Counter

	// NewPerInstanceCounter adds a Per instance Counter with optional Tags to a store, or a scope.
	NewPerInstanceCounter(name string, tags map[string]string) Counter

	// NewGauge adds a Gauge to a store, or a scope.
	NewGauge(name string) Gauge

	// NewGaugeWithTags adds a Gauge with Tags to a store, or a scope.
	NewGaugeWithTags(name string, tags map[string]string) Gauge

	// NewPerInstanceGauge adds a Per instance Gauge with optional Tags to a store, or a scope.
	NewPerInstanceGauge(name string, tags map[string]string) Gauge

	// NewTimer adds a Timer to a store, or a scope that uses microseconds as its unit.
	NewTimer(name string) Timer

	// NewTimerWithTags adds a Timer with Tags to a store, or a scope with Tags that uses microseconds as its unit.
	NewTimerWithTags(name string, tags map[string]string) Timer

	// NewPerInstanceTimer adds a Per instance Timer with optional Tags to a store, or a scope that uses microseconds as its unit.
	NewPerInstanceTimer(name string, tags map[string]string) Timer

	// NewMilliTimer adds a Timer to a store, or a scope that uses milliseconds as its unit.
	NewMilliTimer(name string) Timer

	// NewMilliTimerWithTags adds a Timer with Tags to a store, or a scope with Tags that uses milliseconds as its unit.
	NewMilliTimerWithTags(name string, tags map[string]string) Timer

	// NewPerInstanceMilliTimer adds a Per instance Timer with optional Tags to a store, or a scope that uses milliseconds as its unit.
	NewPerInstanceMilliTimer(name string, tags map[string]string) Timer
}

// A Counter is an always incrementing stat.
type Counter interface {
	// Add increments the Counter by the argument's value.
	Add(uint64)

	// Inc increments the Counter by 1.
	Inc()

	// Set sets an internal counter value which will be written in the next flush.
	// Its use is discouraged as it may break the counter's "always incrementing" semantics.
	Set(uint64)

	// String returns the current value of the Counter as a string.
	String() string

	// Value returns the current value of the Counter as a uint64.
	Value() uint64
}

// A Gauge is a stat that can increment and decrement.
type Gauge interface {
	// Add increments the Gauge by the argument's value.
	Add(uint64)

	// Sub decrements the Gauge by the argument's value.
	Sub(uint64)

	// Inc increments the Gauge by 1.
	Inc()

	// Dec decrements the Gauge by 1.
	Dec()

	// Set sets the Gauge to a value.
	Set(uint64)

	// String returns the current value of the Gauge as a string.
	String() string

	// Value returns the current value of the Gauge as a uint64.
	Value() uint64
}

// A Timer is used to flush timing statistics.
type Timer interface {
	// AddValue flushs the timer with the argument's value.
	AddValue(float64)

	// AddDuration emits the duration as a timing measurement.
	AddDuration(time.Duration)

	// AllocateSpan allocates a Timespan.
	AllocateSpan() Timespan
}

// A Timespan is used to measure spans of time.
// They measure time from the time they are allocated by a Timer with
//
//	AllocateSpan()
//
// until they call
//
//	Complete()
//
// or
//
//	CompleteWithDuration(time.Duration)
//
// When either function is called the timespan is flushed.
// When Complete is called the timespan is flushed.
//
// A Timespan can be flushed at function
// return by calling Complete with golang's defer statement.
type Timespan interface {
	// End the Timespan and flush it.
	Complete() time.Duration

	// End the Timespan and flush it. Adds additional time.Duration to the measured time
	CompleteWithDuration(time.Duration)
}

// A StatGenerator can be used to programatically generate stats.
// StatGenerators are added to a store via
//
//	AddStatGenerator(StatGenerator)
//
// An example is https://github.com/lyft/gostats/blob/master/runtime.go.
type StatGenerator interface {
	// Runs the StatGenerator to generate Stats.
	GenerateStats()
}

// NewStore returns an Empty store that flushes to Sink passed as an argument.
// Note: the export argument is unused.
func NewStore(sink Sink, _ bool) Store {
	return &statStore{sink: sink}
}

// NewDefaultStore returns a Store with a TCP statsd sink, and a running flush timer.
func NewDefaultStore() Store {
	var newStore Store
	settings := GetSettings()
	if !settings.UseStatsd {
		if settings.LoggingSinkDisabled {
			newStore = NewStore(NewNullSink(), false)
		} else {
			newStore = NewStore(NewLoggingSink(), false)
		}
		go newStore.Start(time.NewTicker(10 * time.Second))
	} else {
		newStore = NewStore(NewTCPStatsdSink(), false)
		go newStore.Start(time.NewTicker(time.Duration(settings.FlushIntervalS) * time.Second))
	}
	return newStore
}

type counter struct {
	currentValue  uint64
	lastSentValue uint64
}

func (c *counter) Add(delta uint64) {
	atomic.AddUint64(&c.currentValue, delta)
}

func (c *counter) Set(value uint64) {
	atomic.StoreUint64(&c.currentValue, value)
}

func (c *counter) Inc() {
	c.Add(1)
}

func (c *counter) Value() uint64 {
	return atomic.LoadUint64(&c.currentValue)
}

func (c *counter) String() string {
	return strconv.FormatUint(c.Value(), 10)
}

func (c *counter) latch() uint64 {
	value := c.Value()
	lastSent := atomic.SwapUint64(&c.lastSentValue, value)
	return value - lastSent
}

type gauge struct {
	value uint64
}

func (c *gauge) String() string {
	return strconv.FormatUint(c.Value(), 10)
}

func (c *gauge) Add(value uint64) {
	atomic.AddUint64(&c.value, value)
}

func (c *gauge) Sub(value uint64) {
	atomic.AddUint64(&c.value, ^(value - 1))
}

func (c *gauge) Inc() {
	c.Add(1)
}

func (c *gauge) Dec() {
	c.Sub(1)
}

func (c *gauge) Set(value uint64) {
	atomic.StoreUint64(&c.value, value)
}

func (c *gauge) Value() uint64 {
	return atomic.LoadUint64(&c.value)
}

type timer struct {
	base time.Duration
	name string
	sink Sink
}

func (t *timer) time(dur time.Duration) {
	t.AddDuration(dur)
}

func (t *timer) AddDuration(dur time.Duration) {
	t.AddValue(float64(dur / t.base))
}

func (t *timer) AddValue(value float64) {
	t.sink.FlushTimer(t.name, value)
}

func (t *timer) AllocateSpan() Timespan {
	return &timespan{timer: t, start: time.Now()}
}

type timespan struct {
	timer *timer
	start time.Time
}

func (ts *timespan) Complete() time.Duration {
	d := time.Since(ts.start)
	ts.timer.time(d)
	return d
}

func (ts *timespan) CompleteWithDuration(value time.Duration) {
	ts.timer.time(value)
}

type statStore struct {
	counters sync.Map
	gauges   sync.Map
	timers   sync.Map

	mu             sync.RWMutex
	statGenerators []StatGenerator

	sink Sink
}

func (s *statStore) StartContext(ctx context.Context, ticker *time.Ticker) {
	for {
		select {
		case <-ctx.Done():
			s.Flush()
			return
		case <-ticker.C:
			s.Flush()
		}
	}
}

func (s *statStore) Start(ticker *time.Ticker) {
	s.StartContext(context.Background(), ticker)
}

func (s *statStore) Flush() {
	s.mu.RLock()
	for _, g := range s.statGenerators {
		g.GenerateStats()
	}
	s.mu.RUnlock()

	s.counters.Range(func(key, v interface{}) bool {
		// do not flush counters that are set to zero
		if value := v.(*counter).latch(); value != 0 {
			s.sink.FlushCounter(key.(string), value)
		}
		return true
	})

	s.gauges.Range(func(key, v interface{}) bool {
		s.sink.FlushGauge(key.(string), v.(*gauge).Value())
		return true
	})

	flushableSink, ok := s.sink.(FlushableSink)
	if ok {
		flushableSink.Flush()
	}
}

func (s *statStore) AddStatGenerator(statGenerator StatGenerator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statGenerators = append(s.statGenerators, statGenerator)
}

func (s *statStore) Store() Store {
	return s
}

func (s *statStore) Scope(name string) Scope {
	return newSubScope(s, name, nil)
}

func (s *statStore) ScopeWithTags(name string, tags map[string]string) Scope {
	return newSubScope(s, name, tags)
}

func (s *statStore) newCounter(serializedName string) *counter {
	if v, ok := s.counters.Load(serializedName); ok {
		return v.(*counter)
	}
	c := new(counter)
	if v, loaded := s.counters.LoadOrStore(serializedName, c); loaded {
		return v.(*counter)
	}
	return c
}

func (s *statStore) NewCounter(name string) Counter {
	return s.newCounter(name)
}

func (s *statStore) NewCounterWithTags(name string, tags map[string]string) Counter {
	return s.newCounter(tagspkg.SerializeTags(name, tags))
}

func (s *statStore) newCounterWithTagSet(name string, tags tagspkg.TagSet) Counter {
	return s.newCounter(tags.Serialize(name))
}

var emptyPerInstanceTags = map[string]string{"_f": "i"}

func (s *statStore) NewPerInstanceCounter(name string, tags map[string]string) Counter {
	if len(tags) == 0 {
		return s.NewCounterWithTags(name, emptyPerInstanceTags)
	}
	if _, found := tags["_f"]; found {
		return s.NewCounterWithTags(name, tags)
	}
	return s.newCounterWithTagSet(name, tagspkg.TagSet(nil).MergePerInstanceTags(tags))
}

func (s *statStore) newGauge(serializedName string) *gauge {
	if v, ok := s.gauges.Load(serializedName); ok {
		return v.(*gauge)
	}
	g := new(gauge)
	if v, loaded := s.gauges.LoadOrStore(serializedName, g); loaded {
		return v.(*gauge)
	}
	return g
}

func (s *statStore) NewGauge(name string) Gauge {
	return s.newGauge(name)
}

func (s *statStore) NewGaugeWithTags(name string, tags map[string]string) Gauge {
	return s.newGauge(tagspkg.SerializeTags(name, tags))
}

func (s *statStore) newGaugeWithTagSet(name string, tags tagspkg.TagSet) Gauge {
	return s.newGauge(tags.Serialize(name))
}

func (s *statStore) NewPerInstanceGauge(name string, tags map[string]string) Gauge {
	if len(tags) == 0 {
		return s.NewGaugeWithTags(name, emptyPerInstanceTags)
	}
	if _, found := tags["_f"]; found {
		return s.NewGaugeWithTags(name, tags)
	}
	return s.newGaugeWithTagSet(name, tagspkg.TagSet(nil).MergePerInstanceTags(tags))
}

func (s *statStore) newTimer(serializedName string, base time.Duration) *timer {
	if v, ok := s.timers.Load(serializedName); ok {
		return v.(*timer)
	}
	t := &timer{name: serializedName, sink: s.sink, base: base}
	if v, loaded := s.timers.LoadOrStore(serializedName, t); loaded {
		return v.(*timer)
	}
	return t
}

func (s *statStore) NewMilliTimer(name string) Timer {
	return s.newTimer(name, time.Millisecond)
}

func (s *statStore) NewMilliTimerWithTags(name string, tags map[string]string) Timer {
	return s.newTimer(tagspkg.SerializeTags(name, tags), time.Millisecond)
}

func (s *statStore) NewTimer(name string) Timer {
	return s.newTimer(name, time.Microsecond)
}

func (s *statStore) NewTimerWithTags(name string, tags map[string]string) Timer {
	return s.newTimer(tagspkg.SerializeTags(name, tags), time.Microsecond)
}

func (s *statStore) newTimerWithTagSet(name string, tags tagspkg.TagSet, base time.Duration) Timer {
	return s.newTimer(tags.Serialize(name), base)
}

func (s *statStore) NewPerInstanceTimer(name string, tags map[string]string) Timer {
	if len(tags) == 0 {
		return s.NewTimerWithTags(name, emptyPerInstanceTags)
	}
	if _, found := tags["_f"]; found {
		return s.NewTimerWithTags(name, tags)
	}
	return s.newTimerWithTagSet(name, tagspkg.TagSet(nil).MergePerInstanceTags(tags), time.Microsecond)
}

func (s *statStore) NewPerInstanceMilliTimer(name string, tags map[string]string) Timer {
	if len(tags) == 0 {
		return s.NewMilliTimerWithTags(name, emptyPerInstanceTags)
	}
	if _, found := tags["_f"]; found {
		return s.NewMilliTimerWithTags(name, tags)
	}
	return s.newTimerWithTagSet(name, tagspkg.TagSet(nil).MergePerInstanceTags(tags), time.Millisecond)
}

type subScope struct {
	registry *statStore
	name     string
	tags     tagspkg.TagSet // read-only and may be shared by multiple subScopes
}

func newSubScope(registry *statStore, name string, tags map[string]string) *subScope {
	return &subScope{registry: registry, name: name, tags: tagspkg.NewTagSet(tags)}
}

func (s *subScope) Scope(name string) Scope {
	return s.ScopeWithTags(name, nil)
}

func (s *subScope) ScopeWithTags(name string, tags map[string]string) Scope {
	return &subScope{
		registry: s.registry,
		name:     joinScopes(s.name, name),
		tags:     s.tags.MergeTags(tags),
	}
}

func (s *subScope) Store() Store {
	return s.registry
}

func (s *subScope) NewCounter(name string) Counter {
	return s.NewCounterWithTags(name, nil)
}

func (s *subScope) NewCounterWithTags(name string, tags map[string]string) Counter {
	return s.registry.newCounterWithTagSet(joinScopes(s.name, name), s.tags.MergeTags(tags))
}

func (s *subScope) NewPerInstanceCounter(name string, tags map[string]string) Counter {
	return s.registry.newCounterWithTagSet(joinScopes(s.name, name),
		s.tags.MergePerInstanceTags(tags))
}

func (s *subScope) NewGauge(name string) Gauge {
	return s.NewGaugeWithTags(name, nil)
}

func (s *subScope) NewGaugeWithTags(name string, tags map[string]string) Gauge {
	return s.registry.newGaugeWithTagSet(joinScopes(s.name, name), s.tags.MergeTags(tags))
}

func (s *subScope) NewPerInstanceGauge(name string, tags map[string]string) Gauge {
	return s.registry.newGaugeWithTagSet(joinScopes(s.name, name),
		s.tags.MergePerInstanceTags(tags))
}

func (s *subScope) NewTimer(name string) Timer {
	return s.NewTimerWithTags(name, nil)
}

func (s *subScope) NewTimerWithTags(name string, tags map[string]string) Timer {
	return s.registry.newTimerWithTagSet(joinScopes(s.name, name), s.tags.MergeTags(tags), time.Microsecond)
}

func (s *subScope) NewPerInstanceTimer(name string, tags map[string]string) Timer {
	return s.registry.newTimerWithTagSet(joinScopes(s.name, name),
		s.tags.MergePerInstanceTags(tags), time.Microsecond)
}

func (s *subScope) NewMilliTimer(name string) Timer {
	return s.NewMilliTimerWithTags(name, nil)
}

func (s *subScope) NewMilliTimerWithTags(name string, tags map[string]string) Timer {
	return s.registry.newTimerWithTagSet(joinScopes(s.name, name), s.tags.MergeTags(tags), time.Millisecond)
}

func (s *subScope) NewPerInstanceMilliTimer(name string, tags map[string]string) Timer {
	return s.registry.newTimerWithTagSet(joinScopes(s.name, name),
		s.tags.MergePerInstanceTags(tags), time.Millisecond)
}

func joinScopes(parent, child string) string {
	return parent + "." + child
}
