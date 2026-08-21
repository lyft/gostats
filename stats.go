package stats

import (
	"context"
	"math"
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
	return &statStore{
		sink:              sink,
		pruneAfterFlushes: pruneAfterFlushesFromEnv(),
	}
}

// pruneAfterFlushesFromEnv reads only GOSTATS_PRUNE_IDLE_SECONDS and
// GOSTATS_FLUSH_INTERVAL_SECONDS - not the full Settings via GetSettings,
// which would make NewStore panic on a malformed value for any gostats
// env var, including ones it has nothing to do with (a service passing
// its own sink specifically to bypass env-driven config could still be
// broken by an unrelated typo, e.g. in STATSD_PORT). A malformed value
// for either of these two falls back to disabled/default rather than
// panicking: failing construction outright over a typo in this one
// opt-in knob is worse than silently not pruning. NewDefaultStore, the
// documented fully-environment-driven constructor, keeps GetSettings's
// existing fail-fast behavior.
//
// The result converts PruneIdleSeconds into a number of flushes, rounding
// up so an idle counter or timer always survives at least the requested
// number of seconds. It assumes the store is flushed at that interval; a
// caller that drives Start with its own ticker of a different period will
// see idle entries pruned after that many of its own flushes instead, not
// after PruneIdleSeconds of wall time.
func pruneAfterFlushesFromEnv() uint32 {
	pruneIdleSeconds, err := envInt("GOSTATS_PRUNE_IDLE_SECONDS", DefaultPruneIdleSeconds)
	if err != nil || pruneIdleSeconds <= 0 {
		return 0
	}
	flushIntervalS, err := envInt("GOSTATS_FLUSH_INTERVAL_SECONDS", DefaultFlushIntervalS)
	if err != nil || flushIntervalS <= 0 {
		flushIntervalS = DefaultFlushIntervalS
	}
	n := (pruneIdleSeconds + flushIntervalS - 1) / flushIntervalS // ceil; both operands >= 1, so n >= 1 always
	if n > math.MaxUint32 {
		n = math.MaxUint32 // clamp: silently wrapping could turn "never prune" into "prune every flush"
	}
	return uint32(n)
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
	// idleFlushes is the number of consecutive flushes for which this
	// counter reported a zero delta. Reset to 0 whenever it reports a
	// nonzero delta. Used to prune idle counters; see statStore.Flush.
	idleFlushes uint32
	// detached is set when statStore.Flush removes this counter from the
	// store for being idle. A subsequent write reattaches it (see
	// maybeRejoin/rejoin) so a reference held past pruning keeps working.
	//
	// Only Flush's active branch ever clears it, and that is only safe
	// because statStore.flushMu serializes Flush() calls against each
	// other - see flushMu's comment for why a second, concurrent Flush()
	// call made a plain boolean here unsafe during development, and why
	// the fix is a mutex around the infrequent Flush() call rather than
	// a lock-free scheme for this flag.
	detached uint32
	// store and name are set at creation only when pruning is enabled
	// (store == nil otherwise), so maybeRejoin is a single pointer
	// comparison - no atomic load - on the hot path when it's not.
	store *statStore
	name  string
}

func (c *counter) Add(delta uint64) {
	atomic.AddUint64(&c.currentValue, delta)
	c.maybeRejoin()
}

func (c *counter) Set(value uint64) {
	atomic.StoreUint64(&c.currentValue, value)
	c.maybeRejoin()
}

// maybeRejoin reattaches the counter to its store if it was pruned for
// being idle. c.store is nil whenever pruning is disabled, making this a
// single non-atomic pointer read on that (the common) path.
func (c *counter) maybeRejoin() {
	if c.store != nil && atomic.LoadUint32(&c.detached) != 0 {
		c.rejoin()
	}
}

// rejoin reattaches a detached counter to its store under its original
// name. If another counter has since claimed that name - because a fresh
// lookup created one while c sat detached - c folds its pending delta into
// that counter and remains detached, so it keeps forwarding on later
// writes instead of leaving them stranded.
//
// rejoin never clears c.detached itself. Only Flush does that (see the
// active branch of its counters.Range below), and only at a point where
// it has just confirmed, via that same Range callback, that c is
// currently and genuinely present in the map. If rejoin cleared it here
// instead, based on this LoadOrStore having (at some point in the past)
// found or made c the map's occupant, a concurrent write could observe c
// still present, conclude it's already attached, and then have Flush
// delete it anyway right after - c would sit outside the map with
// detached == 0, and maybeRejoin would never fire again: a permanent
// orphan. Not touching the flag here closes that window entirely rather
// than narrowing it.
func (c *counter) rejoin() {
	v, loaded := c.store.counters.LoadOrStore(c.name, c)
	if !loaded {
		return
	}
	if other := v.(*counter); other != c {
		other.Add(c.latch())
	}
	// other == c: a concurrent write already reattached us. Nothing to do.
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

// latch reports the delta since the last latch and advances lastSentValue
// to the current value. It was only ever called from the single Flush
// goroutine's Range before rejoin's forwarding path existed (see rejoin):
// that path calls c.latch() from whatever goroutine is writing to a
// permanently-detached, forwarding counter, so latch must tolerate
// concurrent callers on the same object.
//
// A plain Load-then-Swap is not safe for that: the two are separate
// atomic operations, and nothing stops a second caller's whole
// read-then-swap from completing in between this caller's read and its
// own swap. A caller whose read is by then stale would swap its smaller
// value in over a larger one already committed - underflowing its own
// delta and regressing lastSentValue backwards, corrupting the next
// caller's delta too. The CAS loop below only commits a read that is
// still current at the moment it commits; a caller that loses the race
// retries against fresh values instead of committing a stale one.
func (c *counter) latch() uint64 {
	for {
		value := c.Value()
		lastSent := atomic.LoadUint64(&c.lastSentValue)
		if value == lastSent {
			return 0
		}
		if atomic.CompareAndSwapUint64(&c.lastSentValue, lastSent, value) {
			return value - lastSent
		}
	}
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
	// active is set whenever AddValue is called and cleared by Flush; it
	// records whether the timer was used during the last flush interval.
	active uint32
	// idleFlushes is the number of consecutive flushes for which this
	// timer was not used. Reset to 0 whenever it is used. Used to prune
	// idle timers; see statStore.Flush.
	idleFlushes uint32
}

func (t *timer) time(dur time.Duration) {
	t.AddDuration(dur)
}

func (t *timer) AddDuration(dur time.Duration) {
	t.AddValue(float64(dur / t.base))
}

func (t *timer) AddValue(value float64) {
	atomic.StoreUint32(&t.active, 1)
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

	// flushMu serializes Flush() calls against each other. Store's own
	// doc comment permits calling Flush both periodically (via
	// Start/StartContext's own goroutine) and on demand ("the store will
	// flush either at the regular interval, or whenever Flush() is
	// called"), so two calls can genuinely run concurrently.
	//
	// The pruning logic below depends on that not happening. An earlier,
	// lock-free version used a generation counter (odd/even + CAS) so a
	// Flush call could tell whether a counter had been re-pruned since it
	// last looked, rather than a single flushMu. It was dropped after
	// three rounds of fixing a race, verifying empirically, and finding
	// the fix had only narrowed the window rather than closed it: winning
	// a CAS, or re-verifying right before a map delete, both still leave
	// a gap - however small - between "check" and "act" that Go's
	// scheduler can land a full, legitimate reattachment cycle inside,
	// because sync.Map has no primitive for "delete this key, but only if
	// some unrelated field on the value still holds a specific number".
	// Each fix for that produced a test failure at a lower rate, not zero,
	// under TestConcurrentFlushesDoNotOrphanCounter.
	//
	// Flush runs periodically (typically every 5-10s), not on the hot
	// Add()/Inc()/Set() path this feature is designed to leave lock-free
	// (see counter.detached and store/name on counter), so a mutex here -
	// and only here - trades a cost that does not exist in the common
	// case (a single ticker-driven Start goroutine never contends this
	// lock at all) for a provable guarantee, rather than continuing to
	// chase a lock-free version with no evidence it terminates.
	flushMu sync.Mutex

	// pruneAfterFlushes is the number of consecutive idle flushes after
	// which a counter or timer is removed from the store. Zero (the zero
	// value, and the default from Settings) disables pruning entirely,
	// preserving today's unbounded-retention behavior.
	//
	// Must not be mutated after construction. Every counter created by
	// this store has its own store/name fields (see counter, newCounter)
	// set based on this field's value AT CREATION time; Flush's prune
	// branch later reads this SAME field to decide whether to delete
	// that counter. Changing it in between would desync the two: a
	// counter created while this was 0 has store == nil, so a later
	// prune (if this were then set nonzero) would delete it with no way
	// for a write to ever rejoin it. NewStore only ever sets this once,
	// so the public API can't hit this - it would take reaching into the
	// unexported statStore directly, which only same-package code can do.
	pruneAfterFlushes uint32
}

var ReservedTagWords = map[string]bool{"asg": true, "az": true, "backend": true, "canary": true, "host": true, "period": true, "region": true, "shard": true, "window": true, "source": true, "project": true, "facet": true, "envoyservice": true}

func (s *statStore) validateTags(tags map[string]string) {
	for k := range tags {
		if _, ok := ReservedTagWords[k]; ok {
			// Keep track of how many times a reserved tag is used
			s.NewCounter("reserved_tag").Inc()
		}
	}
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

// Internal observability for the pruning mechanism above, both gated
// behind pruning being enabled (see the "if s.pruneAfterFlushes > 0"
// block in Flush below) - emitting them unconditionally would add three
// gauge series to every store's wire output, including stores that never
// opt in, and breaks tests that assert exact sink output. gostats.tracked
// reports live map sizes; gostats.pruned reports eviction counts, for
// alerting on churn once pruning is enabled.
const (
	trackedCountersName = "gostats.tracked.__type=counter"
	trackedGaugesName   = "gostats.tracked.__type=gauge"
	trackedTimersName   = "gostats.tracked.__type=timer"
	prunedCountersName  = "gostats.pruned.__type=counter"
	prunedTimersName    = "gostats.pruned.__type=timer"
)

func (s *statStore) Flush() {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	s.mu.RLock()
	for _, g := range s.statGenerators {
		g.GenerateStats()
	}
	s.mu.RUnlock()

	var liveCounters, prunedCounters int64
	s.counters.Range(func(key, v interface{}) bool {
		c := v.(*counter)
		// do not flush counters that are set to zero
		if value := c.latch(); value != 0 {
			s.sink.FlushCounter(key.(string), value)
			atomic.StoreUint32(&c.idleFlushes, 0)
			// The only place detached is ever cleared: right here,
			// having just confirmed via this Range callback that c is
			// currently in the map. Safe only because flushMu means no
			// other Flush() call can be doing the same thing
			// concurrently - see flushMu's comment. See the comment on
			// rejoin for why clearing it from a write's rejoin() instead
			// would be unsafe regardless of that.
			atomic.StoreUint32(&c.detached, 0)
			liveCounters++
			return true
		}
		if s.pruneAfterFlushes == 0 || atomic.AddUint32(&c.idleFlushes, 1) < s.pruneAfterFlushes {
			liveCounters++
			return true
		}
		// Delete before marking detached. Not load-bearing for
		// correctness on its own - rejoin() never clears detached (see
		// its comment), so a write that races in either order still
		// resolves correctly - but it shrinks the window where the flag
		// says detached while c is still actually present in the map.
		s.counters.Delete(key)
		atomic.StoreUint32(&c.detached, 1)
		// A write may have raced between the original latch() above and
		// the delete just now. Catch it here instead of leaving it
		// stranded until c's next write, which may never come.
		if v := c.latch(); v != 0 {
			s.sink.FlushCounter(key.(string), v)
			atomic.StoreUint32(&c.idleFlushes, 0)
			c.rejoin()
			liveCounters++
		} else {
			prunedCounters++
		}
		return true
	})

	var liveTimers, prunedTimers int64
	s.timers.Range(func(key, v interface{}) bool {
		t := v.(*timer)
		if atomic.SwapUint32(&t.active, 0) != 0 {
			atomic.StoreUint32(&t.idleFlushes, 0)
			liveTimers++
			return true
		}
		if s.pruneAfterFlushes == 0 || atomic.AddUint32(&t.idleFlushes, 1) < s.pruneAfterFlushes {
			liveTimers++
			return true
		}
		s.timers.Delete(key)
		prunedTimers++
		return true
	})

	var liveGauges int64
	s.gauges.Range(func(key, v interface{}) bool {
		s.sink.FlushGauge(key.(string), v.(*gauge).Value())
		liveGauges++
		return true
	})

	// Gated behind pruning being enabled: emitting these unconditionally
	// would add three new gauge series to the wire output of every store
	// in the fleet, including the vast majority that never opt in. The
	// ticket's ask is for evictions to be observable, which only applies
	// once eviction is happening at all.
	if s.pruneAfterFlushes > 0 {
		s.sink.FlushGauge(trackedCountersName, uint64(liveCounters))
		s.sink.FlushGauge(trackedGaugesName, uint64(liveGauges))
		s.sink.FlushGauge(trackedTimersName, uint64(liveTimers))
		if prunedCounters != 0 {
			s.sink.FlushCounter(prunedCountersName, uint64(prunedCounters))
		}
		if prunedTimers != 0 {
			s.sink.FlushCounter(prunedTimersName, uint64(prunedTimers))
		}
	}

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
	s.validateTags(tags)
	return newSubScope(s, name, tags)
}

func (s *statStore) newCounter(serializedName string) *counter {
	if v, ok := s.counters.Load(serializedName); ok {
		return v.(*counter)
	}
	c := new(counter)
	if s.pruneAfterFlushes > 0 {
		c.store = s
		c.name = serializedName
	}
	if v, loaded := s.counters.LoadOrStore(serializedName, c); loaded {
		return v.(*counter)
	}
	return c
}

func (s *statStore) NewCounter(name string) Counter {
	return s.newCounter(name)
}

func (s *statStore) NewCounterWithTags(name string, tags map[string]string) Counter {
	s.validateTags(tags)
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
	s.validateTags(tags)
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
	s.validateTags(tags)
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
	s.validateTags(tags)
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
	s.validateTags(tags)
	return s.newTimer(tagspkg.SerializeTags(name, tags), time.Millisecond)
}

func (s *statStore) NewTimer(name string) Timer {
	return s.newTimer(name, time.Microsecond)
}

func (s *statStore) NewTimerWithTags(name string, tags map[string]string) Timer {
	s.validateTags(tags)
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
	s.validateTags(tags)
	return s.newTimerWithTagSet(name, tagspkg.TagSet(nil).MergePerInstanceTags(tags), time.Microsecond)
}

func (s *statStore) NewPerInstanceMilliTimer(name string, tags map[string]string) Timer {
	if len(tags) == 0 {
		return s.NewMilliTimerWithTags(name, emptyPerInstanceTags)
	}
	if _, found := tags["_f"]; found {
		return s.NewMilliTimerWithTags(name, tags)
	}
	s.validateTags(tags)
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
	s.registry.validateTags(tags)
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
	s.registry.validateTags(tags)
	return s.registry.newTimerWithTagSet(joinScopes(s.name, name),
		s.tags.MergePerInstanceTags(tags), time.Millisecond)
}

func joinScopes(parent, child string) string {
	return parent + "." + child
}
