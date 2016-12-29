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

type Store interface {
	Flush()
	Start(*time.Ticker)
	AddStatGenerator(StatGenerator)
	Scope
}

type Scope interface {
	Scope(name string) Scope
	Store() Store
	NewCounter(name string) Counter
	NewCounterWithTags(name string, tags map[string]string) Counter
	NewGauge(name string) Gauge
	NewGaugeWithTags(name string, tags map[string]string) Gauge
	NewTimer(name string) Timer
	NewTimerWithTags(name string, tags map[string]string) Timer
}

type Counter interface {
	Add(uint64)
	Inc()
	Set(uint64)
	String() string
	Value() uint64
}

type Gauge interface {
	Add(uint64)
	Sub(uint64)
	Inc()
	Dec()
	Set(uint64)
	String() string
	Value() uint64
}

type Timer interface {
	AddValue(float64)
	AllocateSpan() Timespan
}

type Timespan interface {
	Complete()
}

type StatGenerator interface {
	GenerateStats()
}

func NewStore(sink Sink, export bool) Store {
	return &statStore{
		counters: make(map[string]*counter),
		gauges:   make(map[string]*gauge),
		timers:   make(map[string]*timer),
		sink:     sink,
		export:   export,
	}
}

func NewDefaultStore() Store {
	var newStore Store
	settings := GetSettings()
	if !settings.UseStatsd {
		logger.Warn("statsd is not in use")
		newStore = NewStore(NewNullSink(), true)
	} else {
		newStore = NewStore(NewTcpStatsdSink(), true)
		go newStore.Start(time.NewTicker(5 * time.Second))
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
	for _, g := range s.statGenerators {
		g.GenerateStats()
	}

	s.Lock()
	defer s.Unlock()

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

func (s subScope) NewGauge(name string) Gauge {
	return s.registry.NewGauge(fmt.Sprintf("%s.%s", s.name, name))
}

func (s subScope) NewGaugeWithTags(name string, tags map[string]string) Gauge {
	return s.registry.NewGaugeWithTags(fmt.Sprintf("%s.%s", s.name, name), tags)
}

func (s subScope) NewTimer(name string) Timer {
	return s.registry.NewTimer(fmt.Sprintf("%s.%s", s.name, name))
}

func (s subScope) NewTimerWithTags(name string, tags map[string]string) Timer {
	return s.registry.NewTimerWithTags(fmt.Sprintf("%s.%s", s.name, name), tags)
}
