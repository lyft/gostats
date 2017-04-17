package stats

import (
	"expvar"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	logger "github.com/Sirupsen/logrus"
)

// A Store holds statistics.
// There are two options when creating a new store:
//  create a store backed by a tcp_sink to statsd
//  s := stats.NewDefaultStore()
//  create a store with a user provided Sink
//  s := stats.NewStore(sink, true)
// Currently that only backing store supported is statsd via a TCP sink, https://github.com/lyft/gostats/blob/master/tcp_sink.go.
// However, implementing other Sinks (https://github.com/lyft/gostats/blob/master/sink.go) should be simple.
//
// A store holds Counters, Gauges, and Timers. You can add unscoped Counters, Gauges, and Timers to the store
// with:
//  s := stats.NewDefaultStore()
//  c := s.New[Counter|Gauge|Timer]("name")
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

	// Start a timer for periodic stat Flushes.
	Start(*time.Ticker)

	// Add a StatGenerator to the Store that programatically generates stats.
	AddStatGenerator(StatGenerator)
	Scope
}

// A Scope namespaces Statistics.
//  store := stats.NewDefaultStore()
//  scope := stats.Scope("service")
//  // the following counter will be emitted at the stats tree rooted at `service`.
//  c := scope.NewCounter("success")
// Additionally you can create subscopes:
//  store := stats.NewDefaultStore()
//  scope := stats.Scope("service")
//  networkScope := scope.Scope("network")
//  // the following counter will be emitted at the stats tree rooted at service.network.
//  c := networkScope.NewCounter("requests")
type Scope interface {
	// Scope creates a subscope.
	Scope(name string) Scope

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

	// NewTimer adds a Timer to a store, or a scope.
	NewTimer(name string) Timer

	// NewTimerWithTags adds a Timer with Tags to a store, or a scope with Tags.
	NewTimerWithTags(name string, tags map[string]string) Timer

	// NewPerInstanceTimer adds a Per instance Timer with optional Tags to a store, or a scope.
	NewPerInstanceTimer(name string, tags map[string]string) Timer
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

	// AllocateSpan allocates a Timespan.
	AllocateSpan() Timespan
}

// A Timespan is used to measure spans of time.
// They measure time from the time they are allocated by a Timer with
//   AllocateSpan()
// until they call
//   Complete().
// When Complete is called the timespan is flushed.
//
// A Timespan can be flushed at function
// return by calling Complete with golang's defer statement.
type Timespan interface {
	// End the Timespan and flush it.
	Complete()
}

// A StatGenerator can be used to programatically generate stats.
// StatGenerators are added to a store via
//  AddStatGenerator(StatGenerator)
// An example is https://github.com/lyft/gostats/blob/master/runtime.go.
type StatGenerator interface {
	// Runs the StatGenerator to generate Stats.
	GenerateStats()
}

// NewStore returns an Empty store that flushes to Sink passed as an argument.
func NewStore(sink Sink, export bool) Store {
	return &statStore{
		counters: make(map[string]*counter),
		gauges:   make(map[string]*gauge),
		timers:   make(map[string]*timer),
		sink:     sink,
		export:   export,
	}
}

// NewDefaultStore returns a Store with a TCP statsd sink, and a running flush timer.
func NewDefaultStore() Store {
	var newStore Store
	settings := GetSettings()
	if !settings.UseStatsd {
		logger.Warn("statsd is not in use")
		newStore = NewStore(NewLoggingSink(), false)
		go newStore.Start(time.NewTicker(10 * time.Second))
	} else {
		newStore = NewStore(NewTcpStatsdSink(), true)
		go newStore.Start(time.NewTicker(time.Duration(settings.FlushIntervalS) * time.Second))
	}
	return newStore
}

type subScope struct {
	registry *statStore
	name     string
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
	return strconv.FormatUint(c.value, 10)
}

func (c *gauge) Add(value uint64) {
	atomic.AddUint64(&c.value, value)
}

func (c *gauge) Sub(value uint64) {
	atomic.AddUint64(&c.value, ^uint64(value-1))
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
	name string
	sink Sink
}

func (t *timer) time(timev time.Duration) {
	t.sink.FlushTimer(t.name, float64(timev/time.Microsecond))
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

func (ts *timespan) Complete() {
	ts.timer.time(time.Now().Sub(ts.start))
}

type statStore struct {
	sync.Mutex
	counters       map[string]*counter
	gauges         map[string]*gauge
	timers         map[string]*timer
	sink           Sink
	statGenerators []StatGenerator
	export         bool
}

func (s *statStore) Flush() {
	s.Lock()
	defer s.Unlock()

	for _, g := range s.statGenerators {
		g.GenerateStats()
	}

	for name, cv := range s.counters {
		value := cv.latch()

		// Skip counters not incremented
		if value == 0 {
			continue
		}

		s.sink.FlushCounter(name, value)
	}

	for name, gv := range s.gauges {
		value := gv.Value()
		s.sink.FlushGauge(name, value)
	}
}

func (s *statStore) Start(ticker *time.Ticker) {
	s.run(ticker)
}

func (s *statStore) AddStatGenerator(statGenerator StatGenerator) {
	s.Lock()
	defer s.Unlock()

	s.statGenerators = append(s.statGenerators, statGenerator)
}

func (s *statStore) run(ticker *time.Ticker) {
	for _ = range ticker.C {
		s.Flush()
	}
}

func (s *statStore) Store() Store {
	return s
}

func (registry *statStore) Scope(name string) Scope {
	return subScope{registry: registry, name: name}
}

func (registry *statStore) NewCounter(name string) Counter {
	registry.Lock()
	defer registry.Unlock()

	counterv, ok := registry.counters[name]
	if ok {
		return counterv
	} else {
		counterv = &counter{}
		registry.counters[name] = counterv
		if registry.export {
			expvar.Publish(name, counterv)
		}
		return counterv
	}
}

func (registry *statStore) NewCounterWithTags(name string, tags map[string]string) Counter {
	serializedTags := serializeTags(tags)
	return registry.NewCounter(fmt.Sprintf("%s%s", name, serializedTags))
}

func (registry *statStore) NewPerInstanceCounter(name string, tags map[string]string) Counter {
	if tags == nil {
		tags = make(map[string]string, 1)
	}

	if _, found := tags["_f"]; !found {
		tags["_f"] = "i"
	}
	serializedTags := serializeTags(tags)
	return registry.NewCounter(fmt.Sprintf("%s%s", name, serializedTags))
}

func (registry *statStore) NewGauge(name string) Gauge {
	registry.Lock()
	defer registry.Unlock()

	gaugev, ok := registry.gauges[name]
	if ok {
		return gaugev
	} else {
		gaugev = &gauge{}
		registry.gauges[name] = gaugev
		if registry.export {
			expvar.Publish(name, gaugev)
		}
		return gaugev
	}
}

func (registry *statStore) NewGaugeWithTags(name string, tags map[string]string) Gauge {
	serializedTags := serializeTags(tags)
	return registry.NewGauge(fmt.Sprintf("%s%s", name, serializedTags))
}

func (registry *statStore) NewPerInstanceGauge(name string, tags map[string]string) Gauge {
	if tags == nil {
		tags = make(map[string]string, 1)
	}

	if _, found := tags["_f"]; !found {
		tags["_f"] = "i"
	}
	serializedTags := serializeTags(tags)
	return registry.NewGauge(fmt.Sprintf("%s%s", name, serializedTags))
}

func (registry *statStore) NewTimer(name string) Timer {
	registry.Lock()
	defer registry.Unlock()

	timerv, ok := registry.timers[name]
	if ok {
		return timerv
	} else {
		timerv = &timer{name: name, sink: registry.sink}
		registry.timers[name] = timerv
		return timerv
	}
}

func (registry *statStore) NewTimerWithTags(name string, tags map[string]string) Timer {
	serializedTags := serializeTags(tags)
	return registry.NewTimer(fmt.Sprintf("%s%s", name, serializedTags))
}

func (registry *statStore) NewPerInstanceTimer(name string, tags map[string]string) Timer {
	if tags == nil {
		tags = make(map[string]string, 1)
	}

	if _, found := tags["_f"]; !found {
		tags["_f"] = "i"
	}
	serializedTags := serializeTags(tags)
	return registry.NewTimer(fmt.Sprintf("%s%s", name, serializedTags))
}

func (s subScope) Scope(name string) Scope {
	return &subScope{registry: s.registry, name: fmt.Sprintf("%s.%s", s.name, name)}
}

func (s subScope) Store() Store {
	return s.registry
}

func (s subScope) NewCounter(name string) Counter {
	return s.registry.NewCounter(fmt.Sprintf("%s.%s", s.name, name))
}

func (s subScope) NewCounterWithTags(name string, tags map[string]string) Counter {
	return s.registry.NewCounterWithTags(fmt.Sprintf("%s.%s", s.name, name), tags)
}

func (s subScope) NewPerInstanceCounter(name string, tags map[string]string) Counter {
	return s.registry.NewPerInstanceCounter(fmt.Sprintf("%s.%s", s.name, name), tags)
}

func (s subScope) NewGauge(name string) Gauge {
	return s.registry.NewGauge(fmt.Sprintf("%s.%s", s.name, name))
}

func (s subScope) NewGaugeWithTags(name string, tags map[string]string) Gauge {
	return s.registry.NewGaugeWithTags(fmt.Sprintf("%s.%s", s.name, name), tags)
}

func (s subScope) NewPerInstanceGauge(name string, tags map[string]string) Gauge {
	return s.registry.NewPerInstanceGauge(fmt.Sprintf("%s.%s", s.name, name), tags)
}

func (s subScope) NewTimer(name string) Timer {
	return s.registry.NewTimer(fmt.Sprintf("%s.%s", s.name, name))
}

func (s subScope) NewTimerWithTags(name string, tags map[string]string) Timer {
	return s.registry.NewTimerWithTags(fmt.Sprintf("%s.%s", s.name, name), tags)
}

func (s subScope) NewPerInstanceTimer(name string, tags map[string]string) Timer {
	return s.registry.NewPerInstanceTimer(fmt.Sprintf("%s.%s", s.name, name), tags)
}
