package stats

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	logger "github.com/sirupsen/logrus"
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
//   Complete()
// or
//   CompleteWithDuration(time.Duration)
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
//  AddStatGenerator(StatGenerator)
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
		logger.Warn("statsd is not in use")
		if settings.LoggingSinkDisabled {
			newStore = NewStore(NewNullSink(), false)
		} else {
			newStore = NewStore(NewLoggingSink(), false)
		}
		go newStore.Start(time.NewTicker(10 * time.Second))
	} else {
		newStore = &statStore{
			sink:         NewTCPStatsdSink(),
			tagPrefix:    settings.StatsdTagPrefix,
			tagSeparator: settings.StatsdTagSeparator}
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

func (t *timer) time(dur time.Duration) {
	t.sink.FlushTimer(t.name, float64(dur/time.Microsecond))
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
	d := time.Now().Sub(ts.start)
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

	genMtx         sync.RWMutex
	statGenerators []StatGenerator

	sink Sink

	tagPrefix    string
	tagSeparator string
}

func (s *statStore) Flush() {
	s.genMtx.RLock()
	for _, g := range s.statGenerators {
		g.GenerateStats()
	}
	s.genMtx.RUnlock()

	s.counters.Range(func(key, v interface{}) bool {
		// Skip counters not incremented
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

func (s *statStore) Start(ticker *time.Ticker) {
	s.run(ticker)
}

func (s *statStore) AddStatGenerator(statGenerator StatGenerator) {
	s.genMtx.Lock()
	s.statGenerators = append(s.statGenerators, statGenerator)
	s.genMtx.Unlock()
}

func (s *statStore) run(ticker *time.Ticker) {
	for range ticker.C {
		s.Flush()
	}
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
	return s.newCounter(s.serializeTags(name, tags))
}

func (s *statStore) newCounterWithTagSet(name string, tags tagSet) Counter {
	return s.newCounter(s.serializeTagSet(name, tags))
}

var emptyPerInstanceTags = map[string]string{"_f": "i"}

func (s *statStore) NewPerInstanceCounter(name string, tags map[string]string) Counter {
	if len(tags) == 0 {
		return s.NewCounterWithTags(name, emptyPerInstanceTags)
	}

	if _, found := tags["_f"]; !found {
		tags["_f"] = "i"
	}

	return s.NewCounterWithTags(name, tags)
}

type statCache struct {
	cache sync.Map
	new   func() interface{}
}

func (s *statCache) LoadOrCreate(key string) interface{} {
	if v, ok := s.cache.Load(key); ok {
		return v
	}
	v, _ := s.cache.LoadOrStore(key, s.new())
	return v
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
	return s.newGauge(s.serializeTags(name, tags))
}

func (s *statStore) newGaugeWithTagSet(name string, tags tagSet) Gauge {
	return s.newGauge(s.serializeTagSet(name, tags))
}

func (s *statStore) NewPerInstanceGauge(name string, tags map[string]string) Gauge {
	if len(tags) == 0 {
		return s.NewGaugeWithTags(name, emptyPerInstanceTags)
	}

	if _, found := tags["_f"]; !found {
		tags["_f"] = "i"
	}

	return s.NewGaugeWithTags(name, tags)
}

func (s *statStore) newTimer(serializedName string) *timer {
	if v, ok := s.timers.Load(serializedName); ok {
		return v.(*timer)
	}
	t := &timer{name: serializedName, sink: s.sink}
	if v, loaded := s.timers.LoadOrStore(serializedName, t); loaded {
		return v.(*timer)
	}
	return t
}

func (s *statStore) NewTimer(name string) Timer {
	return s.newTimer(name)
}

func (s *statStore) NewTimerWithTags(name string, tags map[string]string) Timer {
	return s.newTimer(s.serializeTags(name, tags))
}

func (s *statStore) newTimerWithTagSet(name string, tags tagSet) Timer {
	return s.newTimer(s.serializeTagSet(name, tags))
}

func (s *statStore) NewPerInstanceTimer(name string, tags map[string]string) Timer {
	if len(tags) == 0 {
		return s.NewTimerWithTags(name, emptyPerInstanceTags)
	}

	if _, found := tags["_f"]; !found {
		tags["_f"] = "i"
	}

	return s.NewTimerWithTags(name, tags)
}

type subScope struct {
	registry *statStore
	name     string
	tags     tagSet // read-only and may be shared by multiple subScopes
}

func newSubScope(registry *statStore, name string, tags map[string]string) *subScope {
	a := make(tagSet, 0, len(tags))
	for k, v := range tags {
		if k != "" && v != "" {
			a = append(a, tagPair{key: k, value: replaceChars(v)})
		}
	}
	a.Sort()
	return &subScope{registry: registry, name: name, tags: a}
}

func (s *subScope) Scope(name string) Scope {
	return s.ScopeWithTags(name, nil)
}

func (s *subScope) ScopeWithTags(name string, tags map[string]string) Scope {
	return &subScope{
		registry: s.registry,
		name:     joinScopes(s.name, name),
		tags:     s.mergeTags(tags),
	}
}

func (s *subScope) Store() Store {
	return s.registry
}

func (s *subScope) NewCounter(name string) Counter {
	return s.NewCounterWithTags(name, nil)
}

func (s *subScope) NewCounterWithTags(name string, tags map[string]string) Counter {
	return s.registry.newCounterWithTagSet(joinScopes(s.name, name), s.mergeTags(tags))
}

func (s *subScope) NewPerInstanceCounter(name string, tags map[string]string) Counter {
	return s.registry.newCounterWithTagSet(joinScopes(s.name, name),
		s.mergePerInstanceTags(tags))
}

func (s *subScope) NewGauge(name string) Gauge {
	return s.NewGaugeWithTags(name, nil)
}

func (s *subScope) NewGaugeWithTags(name string, tags map[string]string) Gauge {
	return s.registry.newGaugeWithTagSet(joinScopes(s.name, name), s.mergeTags(tags))
}

func (s *subScope) NewPerInstanceGauge(name string, tags map[string]string) Gauge {
	return s.registry.newGaugeWithTagSet(joinScopes(s.name, name),
		s.mergePerInstanceTags(tags))
}

func (s *subScope) NewTimer(name string) Timer {
	return s.NewTimerWithTags(name, nil)
}

func (s *subScope) NewTimerWithTags(name string, tags map[string]string) Timer {
	return s.registry.newTimerWithTagSet(joinScopes(s.name, name), s.mergeTags(tags))
}

func (s *subScope) NewPerInstanceTimer(name string, tags map[string]string) Timer {
	return s.registry.newTimerWithTagSet(joinScopes(s.name, name),
		s.mergePerInstanceTags(tags))
}

func joinScopes(parent, child string) string {
	return parent + "." + child
}

// mergeOneTag is an optimized for inserting 1 tag and will panic otherwise.
func (s *subScope) mergeOneTag(tags map[string]string) tagSet {
	if len(tags) != 1 {
		panic("invalid usage")
	}
	var p tagPair
	for k, v := range tags {
		p = tagPair{key: k, value: replaceChars(v)}
		break
	}
	if p.key == "" || p.value == "" {
		return s.tags
	}
	a := make(tagSet, len(s.tags), len(s.tags)+1)
	copy(a, s.tags)
	return a.Insert(p)
}

// mergeTags returns a tagSet that is the union of subScope's tags and the
// provided tags map. If any keys overlap the values from the provided map
// are used.
func (s *subScope) mergeTags(tags map[string]string) tagSet {
	switch len(tags) {
	case 0:
		return s.tags
	case 1:
		// optimize for the common case of there only being one tag
		return s.mergeOneTag(tags)
	default:
		// write tags to the end of the scratch slice
		scratch := make(tagSet, len(s.tags)+len(tags))
		a := scratch[len(s.tags):]
		i := 0
		for k, v := range tags {
			if k != "" && v != "" {
				a[i] = tagPair{key: k, value: replaceChars(v)}
				i++
			}
		}
		a = a[:i]
		a.Sort()

		if len(s.tags) == 0 {
			return a
		}
		return mergeTagSets(s.tags, a, scratch)
	}
}

// mergePerInstanceTags returns a tagSet that is the union of subScope's
// tags and the provided tags map with. If any keys overlap the values from
// the provided map are used.
//
// The returned tagSet will have a per-instance key ("_f") and if neither the
// subScope or tags have this key it's value will be the default per-instance
// value ("i").
//
// The method does not optimize for the case where there is only one tag
// because it is used less frequently.
func (s *subScope) mergePerInstanceTags(tags map[string]string) tagSet {
	if len(tags) == 0 {
		if s.tags.Contains("_f") {
			return s.tags
		}
		// create copy with the per-instance tag
		a := make(tagSet, len(s.tags), len(s.tags)+1)
		copy(a, s.tags)
		return a.Insert(tagPair{key: "_f", value: "i"})
	}

	// write tags to the end of scratch slice
	scratch := make(tagSet, len(s.tags)+len(tags)+1)
	a := scratch[len(s.tags):]
	i := 0
	for k, v := range tags {
		if k != "" && v != "" {
			a[i] = tagPair{key: k, value: replaceChars(v)}
			i++
		}
	}
	// add the default per-instance tag if not present
	if tags["_f"] == "" && !s.tags.Contains("_f") {
		a[i] = tagPair{key: "_f", value: "i"}
		i++
	}
	a = a[:i]
	a.Sort()

	if len(s.tags) == 0 {
		return a
	}
	return mergeTagSets(s.tags, a, scratch)
}
