package stats

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	tagspkg "github.com/lyft/gostats/internal/tags"
	"github.com/lyft/gostats/mock"
)

// Ensure flushing and adding generators does not race
func TestStats(_ *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	scope := store.Scope("runtime")
	g := NewRuntimeStats(scope)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		store.AddStatGenerator(g)
		store.NewCounter("test")
		store.Flush()
		wg.Done()
	}()

	go func() {
		store.AddStatGenerator(g)
		store.NewCounter("test")
		store.Flush()
		wg.Done()
	}()

	wg.Wait()
}

// TestStatsStartContext ensures that a cancelled context cancels a
// flushing goroutine.
func TestStatsStartContext(_ *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	ctx, cancel := context.WithCancel(context.Background())
	tick := time.NewTicker(1 * time.Minute)

	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		store.StartContext(ctx, tick)
	}()

	// now we cancel, and its ok to do this at any point - the
	// goroutine above could have started or not started, either case
	// is ok.
	cancel()

	wg.Wait()
}

// Ensure we create a counter and increment it for reserved tags
func TestValidateTags(t *testing.T) {
	// Ensure we don't create a counter without reserved tags
	sink := &testStatSink{}
	store := NewStore(sink, true)
	store.NewCounter("test").Inc()
	store.Flush()

	expected := "test:1|c"
	output := sink.record
	if !strings.Contains(output, expected) && !strings.Contains(output, "reserved_tag") {
		t.Errorf("Expected without reserved tags: '%s' Got: '%s'", expected, output)
	}

	// A reserved tag should trigger adding the reserved_tag counter
	sink = &testStatSink{}
	store = NewStore(sink, true)
	store.NewCounterWithTags("test", map[string]string{"host": "i"}).Inc()
	store.Flush()

	expected = "test.__host=i:1|c"
	expectedReservedTag := "reserved_tag:1|c"
	output = sink.record
	if !strings.Contains(output, expected) && !strings.Contains(output, expectedReservedTag) {
		t.Errorf("Expected: '%s' and '%s', In: '%s'", expected, expectedReservedTag, output)
	}
}

// Ensure timers and timespans are working
func TestTimer(t *testing.T) {
	testDuration := time.Duration(9800000)
	sink := &testStatSink{}
	store := NewStore(sink, true)
	store.NewTimer("test").AllocateSpan().CompleteWithDuration(testDuration)
	store.Flush()

	expected := "test:9800.000000|ms"
	timer := sink.record
	if !strings.Contains(timer, expected) {
		t.Error("wanted timer value of test:9800.000000|ms, got", timer)
	}
}

// Ensure millitimers and timespans are working
func TestMilliTimer(t *testing.T) {
	testDuration := 420 * time.Millisecond
	sink := &testStatSink{}
	store := NewStore(sink, true)
	store.NewMilliTimer("test").AllocateSpan().CompleteWithDuration(testDuration)
	store.Flush()

	expected := "test:420.000000|ms"
	timer := sink.record
	if !strings.Contains(timer, expected) {
		t.Error("wanted timer value of test:420.000000|ms, got", timer)
	}
}

// Ensure 0 counters are not flushed
func TestZeroCounters(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)
	store.NewCounter("test")
	store.Flush()

	expected := ""
	counter := sink.record
	if counter != expected {
		t.Errorf("wanted %q got %q", expected, counter)
	}
}

func randomString(tb testing.TB, size int) string {
	b := make([]byte, hex.DecodedLen(size))
	if _, err := crand.Read(b); err != nil {
		tb.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func randomTagSet(t testing.TB, valPrefix string, size int) tagspkg.TagSet {
	s := make(tagspkg.TagSet, size)
	for i := 0; i < len(s); i++ {
		s[i] = tagspkg.NewTag(randomString(t, 32), fmt.Sprintf("%s%d", valPrefix, i))
	}
	s.Sort()
	return s
}

func TestNewSubScope(t *testing.T) {
	s := randomTagSet(t, "x_", 20)
	for i := range s {
		s[i].Value += "|" // add an invalid char
	}
	m := make(map[string]string)
	for _, p := range s {
		m[p.Key] = p.Value
	}
	scope := newSubScope(nil, "name", m)

	expected := make(tagspkg.TagSet, len(s))
	for i, p := range s {
		expected[i] = tagspkg.NewTag(p.Key, p.Value)
	}

	if !reflect.DeepEqual(scope.tags, expected) {
		t.Errorf("tags are not sorted by key: %+v", s)
	}
	for i, p := range expected {
		s := tagspkg.ReplaceChars(p.Value)
		if p.Value != s {
			t.Errorf("failed to replace invalid chars: %d: %+v", i, p)
		}
	}
	if scope.name != "name" {
		t.Errorf("wrong scope name: %s", scope.name)
	}
}

// Test that we never modify the tags map that is passed in
func TestTagMapNotModified(t *testing.T) {
	type TagMethod func(scope Scope, name string, tags map[string]string)

	copyTags := func(tags map[string]string) map[string]string {
		orig := make(map[string]string, len(tags))
		for k, v := range tags {
			orig[k] = v
		}
		return orig
	}

	scopeGenerators := map[string]func() Scope{
		"statStore": func() Scope { return &statStore{} },
		"subScope":  func() Scope { return newSubScope(&statStore{}, "name", nil) },
	}

	methodTestCases := map[string]TagMethod{
		"ScopeWithTags": func(scope Scope, name string, tags map[string]string) {
			scope.ScopeWithTags(name, tags)
		},
		"NewCounterWithTags": func(scope Scope, name string, tags map[string]string) {
			scope.NewCounterWithTags(name, tags)
		},
		"NewPerInstanceCounter": func(scope Scope, name string, tags map[string]string) {
			scope.NewPerInstanceCounter(name, tags)
		},
		"NewGaugeWithTags": func(scope Scope, name string, tags map[string]string) {
			scope.NewGaugeWithTags(name, tags)
		},
		"NewPerInstanceGauge": func(scope Scope, name string, tags map[string]string) {
			scope.NewPerInstanceGauge(name, tags)
		},
		"NewTimerWithTags": func(scope Scope, name string, tags map[string]string) {
			scope.NewTimerWithTags(name, tags)
		},
		"NewPerInstanceTimer": func(scope Scope, name string, tags map[string]string) {
			scope.NewPerInstanceTimer(name, tags)
		},
	}

	tagsTestCases := []map[string]string{
		{}, // empty
		{
			"": "invalid_key",
		},
		{
			"invalid_value": "",
		},
		{
			"":              "invalid_key",
			"invalid_value": "",
		},
		{
			"_f": "i",
		},
		{
			"":              "invalid_key",
			"invalid_value": "",
			"_f":            "i",
		},
		{
			"":              "invalid_key",
			"invalid_value": "",
			"_f":            "value",
			"1":             "1",
		},
		{
			"":              "invalid_key",
			"invalid_value": "",
			"1":             "1",
			"2":             "2",
			"3":             "3",
		},
	}

	for scopeName, newScope := range scopeGenerators {
		for methodName, method := range methodTestCases {
			t.Run(scopeName+"."+methodName, func(t *testing.T) {
				for _, orig := range tagsTestCases {
					tags := copyTags(orig)
					method(newScope(), "test", tags)
					if !reflect.DeepEqual(tags, orig) {
						t.Errorf("modified input map: %+v want: %+v", tags, orig)
					}
				}
			})
		}
	}
}

func TestPerInstanceStats(t *testing.T) {
	testCases := []struct {
		expected string
		tags     map[string]string
	}{
		{
			expected: "name.___f=i",
			tags:     map[string]string{}, // empty
		},
		{
			expected: "name.___f=i",
			tags: map[string]string{
				"": "invalid_key",
			},
		},
		{
			expected: "name.___f=i",
			tags: map[string]string{
				"invalid_value": "",
			},
		},
		{
			expected: "name.___f=i",
			tags: map[string]string{
				"_f": "i",
			},
		},
		{
			expected: "name.___f=xxx",
			tags: map[string]string{
				"_f": "xxx",
			},
		},
		{
			expected: "name.___f=xxx",
			tags: map[string]string{
				"":   "invalid_key",
				"_f": "xxx",
			},
		},
		{
			expected: "name.___f=xxx",
			tags: map[string]string{
				"invalid_value": "",
				"_f":            "xxx",
			},
		},
		{
			expected: "name.___f=xxx",
			tags: map[string]string{
				"invalid_value": "",
				"":              "invalid_key",
				"_f":            "xxx",
			},
		},
		{
			expected: "name.__1=1.___f=xxx",
			tags: map[string]string{
				"invalid_value": "",
				"":              "invalid_key",
				"_f":            "xxx",
				"1":             "1",
			},
		},
		{
			expected: "name.__1=1.___f=i",
			tags: map[string]string{
				"1": "1",
			},
		},
		{
			expected: "name.__1=1.__2=2.___f=i",
			tags: map[string]string{
				"1": "1",
				"2": "2",
			},
		},
	}

	testPerInstanceMethods := func(t *testing.T, setupScope func(Scope) Scope) {
		for _, x := range testCases {
			sink := mock.NewSink()
			scope := setupScope(&statStore{sink: sink})

			scope.NewPerInstanceCounter("name", x.tags).Inc()
			scope.NewPerInstanceGauge("name", x.tags).Inc()
			scope.NewPerInstanceTimer("name", x.tags).AddValue(1)
			scope.Store().Flush()

			for key := range sink.Counters() {
				if key != x.expected {
					t.Errorf("Counter (%+v): got: %q want: %q", x, key, x.expected)
				}
				break
			}

			for key := range sink.Gauges() {
				if key != x.expected {
					t.Errorf("Gauge (%+v): got: %q want: %q", x, key, x.expected)
				}
				break
			}

			for key := range sink.Timers() {
				if key != x.expected {
					t.Errorf("Timer (%+v): got: %q want: %q", x, key, x.expected)
				}
				break
			}
		}
	}

	t.Run("StatsStore", func(t *testing.T) {
		testPerInstanceMethods(t, func(scope Scope) Scope { return scope })
	})

	t.Run("SubScope", func(t *testing.T) {
		// Add sub-scope prefix to the name
		for i, x := range testCases {
			testCases[i].expected = "x." + x.expected
		}

		testPerInstanceMethods(t, func(scope Scope) Scope {
			return scope.Scope("x")
		})
	})
}

// mapLen returns the number of entries in a sync.Map. It may return a
// stale/incoherent count if m is modified concurrently, which is fine for
// these single-goroutine tests.
func mapLen(m *sync.Map) int {
	n := 0
	m.Range(func(_, _ interface{}) bool {
		n++
		return true
	})
	return n
}

// A counter or timer that goes unwritten for pruneAfterFlushes consecutive
// flushes is removed from the store to bound memory growth from
// high-cardinality tags (see settings.go: GOSTATS_PRUNE_IDLE_SECONDS).
func TestStatsStorePruneIdleCounters(t *testing.T) {
	sink := mock.NewSink()
	const threshold = 3
	s := &statStore{sink: sink, pruneAfterFlushes: threshold}

	const n = 8
	for i := 0; i < n; i++ {
		s.NewCounter("counter_" + strconv.Itoa(i))
	}
	if got := mapLen(&s.counters); got != n {
		t.Fatalf("len(counters) = %d, want %d", got, n)
	}

	// Never written after creation, so every flush counts as idle. The
	// first threshold-1 flushes must not prune.
	for i := 0; i < threshold-1; i++ {
		s.Flush()
		if got := mapLen(&s.counters); got != n {
			t.Fatalf("after flush %d/%d: len(counters) = %d, want %d (not yet pruned)", i+1, threshold, got, n)
		}
	}
	// The threshold-th idle flush prunes.
	s.Flush()
	if got := mapLen(&s.counters); got != 0 {
		t.Fatalf("after flush %d: len(counters) = %d, want 0 (pruned)", threshold, got)
	}
}

func TestStatsStorePruneIdleTimers(t *testing.T) {
	sink := mock.NewSink()
	const threshold = 3
	s := &statStore{sink: sink, pruneAfterFlushes: threshold}

	const n = 8
	for i := 0; i < n; i++ {
		s.NewTimer("timer_" + strconv.Itoa(i))
	}
	if got := mapLen(&s.timers); got != n {
		t.Fatalf("len(timers) = %d, want %d", got, n)
	}

	for i := 0; i < threshold-1; i++ {
		s.Flush()
		if got := mapLen(&s.timers); got != n {
			t.Fatalf("after flush %d/%d: len(timers) = %d, want %d (not yet pruned)", i+1, threshold, got, n)
		}
	}
	s.Flush()
	if got := mapLen(&s.timers); got != 0 {
		t.Fatalf("after flush %d: len(timers) = %d, want 0 (pruned)", threshold, got)
	}
}

// A counter or timer written at least once every pruneAfterFlushes flushes
// must never be pruned.
func TestStatsStorePruneSkipsActiveMetrics(t *testing.T) {
	sink := mock.NewSink()
	const threshold = 3
	s := &statStore{sink: sink, pruneAfterFlushes: threshold}

	c := s.NewCounter("active_counter")
	tm := s.NewTimer("active_timer")

	for i := 0; i < threshold*3; i++ {
		c.Inc()
		tm.AddValue(1)
		s.Flush()
	}
	if got := mapLen(&s.counters); got != 1 {
		t.Errorf("len(counters) = %d, want 1 (active counter must survive)", got)
	}
	if got := mapLen(&s.timers); got != 1 {
		t.Errorf("len(timers) = %d, want 1 (active timer must survive)", got)
	}
}

// Pruning is opt-in: the zero-value statStore{} (pruneAfterFlushes == 0)
// must never prune, matching today's unbounded behavior exactly.
func TestStatsStorePruneDisabledByDefault(t *testing.T) {
	sink := mock.NewSink()
	s := &statStore{sink: sink} // pruneAfterFlushes zero value: disabled

	s.NewCounter("foo")
	s.NewTimer("bar")

	for i := 0; i < 50; i++ {
		s.Flush()
	}
	if got := mapLen(&s.counters); got != 1 {
		t.Errorf("len(counters) = %d, want 1 (pruning disabled)", got)
	}
	if got := mapLen(&s.timers); got != 1 {
		t.Errorf("len(timers) = %d, want 1 (pruning disabled)", got)
	}
}

// A held Counter reference must not silently stop reporting after being
// pruned for going idle - the next write must reattach it to the store.
// This is what makes pruning safe for the common pattern of resolving a
// Counter once and holding it in a struct field for the process lifetime.
func TestPrunedCounterRejoinsOnWrite(t *testing.T) {
	sink := mock.NewSink()
	const threshold = 3
	s := &statStore{sink: sink, pruneAfterFlushes: threshold}

	c := s.NewCounter("reattaches")

	// Age it out.
	for i := 0; i < threshold; i++ {
		s.Flush()
	}
	if got := mapLen(&s.counters); got != 0 {
		t.Fatalf("len(counters) = %d, want 0 (should have been pruned)", got)
	}

	// The held reference is written to again after being pruned.
	c.Inc()
	s.Flush()

	if got, ok := sink.LoadCounter("reattaches"); !ok || got != 1 {
		t.Errorf("LoadCounter(%q) = (%d, %v), want (1, true)", "reattaches", got, ok)
	}
}

// Rejoin must work via Set(), not just Add()/Inc() - they update
// currentValue through separate call sites.
func TestPrunedCounterRejoinsOnSet(t *testing.T) {
	sink := mock.NewSink()
	const threshold = 3
	s := &statStore{sink: sink, pruneAfterFlushes: threshold}

	c := s.NewCounter("reattaches_via_set")
	for i := 0; i < threshold; i++ {
		s.Flush()
	}
	if got := mapLen(&s.counters); got != 0 {
		t.Fatalf("len(counters) = %d, want 0 (should have been pruned)", got)
	}

	c.Set(5)
	s.Flush()

	if got, ok := sink.LoadCounter("reattaches_via_set"); !ok || got != 5 {
		t.Errorf("LoadCounter(%q) = (%d, %v), want (5, true)", "reattaches_via_set", got, ok)
	}
}

// If a fresh lookup creates a new counter under the same name while the
// original, still-held counter sits detached, the original must fold its
// pending delta into the new one on its next write rather than losing it.
func TestPrunedCounterRejoinLosesRace(t *testing.T) {
	sink := mock.NewSink()
	const threshold = 3
	s := &statStore{sink: sink, pruneAfterFlushes: threshold}

	held := s.NewCounter("contested")
	for i := 0; i < threshold; i++ {
		s.Flush()
	}
	if got := mapLen(&s.counters); got != 0 {
		t.Fatalf("len(counters) = %d, want 0 (should have been pruned)", got)
	}

	// A fresh, unrelated lookup recreates the name before the held
	// reference writes again.
	fresh := s.NewCounter("contested")
	fresh.Inc()

	// The original, still-held reference is written to after that.
	held.Inc()
	s.Flush()

	if got, ok := sink.LoadCounter("contested"); !ok || got != 2 {
		t.Errorf("LoadCounter(%q) = (%d, %v), want (2, true)", "contested", got, ok)
	}
}

// A held Timer reference must keep working after being pruned. Timers are
// stateless - AddValue writes straight to the sink - so this requires no
// rejoin machinery at all, unlike counters.
func TestPrunedTimerStillEmits(t *testing.T) {
	sink := mock.NewSink()
	const threshold = 3
	s := &statStore{sink: sink, pruneAfterFlushes: threshold}

	tm := s.NewTimer("prunable_timer")
	for i := 0; i < threshold; i++ {
		s.Flush()
	}
	if got := mapLen(&s.timers); got != 0 {
		t.Fatalf("len(timers) = %d, want 0 (should have been pruned)", got)
	}

	tm.AddValue(42)

	if got, ok := sink.LoadTimer("prunable_timer"); !ok || got != 42 {
		t.Errorf("LoadTimer(%q) = (%v, %v), want (42, true)", "prunable_timer", got, ok)
	}
}

// Concurrent increments must never be lost to a race between a writer and
// the flush goroutine pruning the same counter for having gone idle.
// threshold=1 makes every idle flush attempt a prune, maximizing exposure
// to the race window between Flush's original latch and the delete.
func TestPrunedCounterRaceWithFlush(t *testing.T) {
	sink := mock.NewSink()
	const threshold = 1
	s := &statStore{sink: sink, pruneAfterFlushes: threshold}

	c := s.NewCounter("raced")

	const goroutines = 8
	const incrementsPerGoroutine = 2000
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < incrementsPerGoroutine; i++ {
				c.Inc()
			}
		}()
	}

	stop := make(chan struct{})
	var flushWg sync.WaitGroup
	flushWg.Add(1)
	go func() {
		defer flushWg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				s.Flush()
			}
		}
	}()

	wg.Wait()
	close(stop)
	flushWg.Wait()

	// One final write+flush guarantees a fully deterministic drain: if c
	// is detached at this point, Inc() reattaches it (nothing else ever
	// looks up this name, so rejoin always wins), and the following
	// Flush reports the total. This removes any timing dependence on
	// exactly when the last racing increment landed.
	c.Inc()
	s.Flush()

	want := uint64(goroutines*incrementsPerGoroutine + 1)
	if got, _ := sink.LoadCounter("raced"); got != want {
		t.Errorf("LoadCounter(%q) = %d, want %d", "raced", got, want)
	}
}

// NewStore derives pruneAfterFlushes from GOSTATS_PRUNE_IDLE_SECONDS and
// the configured flush interval, rounding up so an idle metric always
// survives at least the requested number of seconds.
func TestNewStorePruneAfterFlushesFromEnv(t *testing.T) {
	tests := []struct {
		name             string
		pruneIdleSecs    string
		flushIntervalS   string
		wantPruneFlushes uint32
	}{
		{"disabled by default", "", "", 0},
		{"exact multiple", "60", "5", 12},
		{"rounds up", "61", "5", 13},
		{"minimum of one flush", "1", "5", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset := testSetenv(t,
				"GOSTATS_PRUNE_IDLE_SECONDS", tt.pruneIdleSecs,
				"GOSTATS_FLUSH_INTERVAL_SECONDS", tt.flushIntervalS,
			)
			defer reset()

			s := NewStore(nullSink{}, false).(*statStore)
			if s.pruneAfterFlushes != tt.wantPruneFlushes {
				t.Errorf("pruneAfterFlushes = %d, want %d", s.pruneAfterFlushes, tt.wantPruneFlushes)
			}
		})
	}
}

// NewStore must not panic on a malformed env var that has nothing to do
// with pruning: it previously called nothing environment-related at all,
// and a caller providing its own sink specifically to bypass env-driven
// config (tests, DI-style setups) shouldn't have that broadened into "any
// gostats env var typo crashes construction" as a side effect of this
// feature. NewDefaultStore, which is documented as the fully
// environment-driven constructor, keeps its existing fail-fast behavior.
func TestNewStoreToleratesUnrelatedMalformedEnvVar(t *testing.T) {
	reset := testSetenv(t, "STATSD_PORT", "not-an-int")
	defer reset()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewStore panicked on an unrelated malformed env var: %v", r)
		}
	}()
	NewStore(nullSink{}, false)
}

// A malformed value for one of the two env vars NewStore actually reads
// must fall back to disabling pruning (for GOSTATS_PRUNE_IDLE_SECONDS) or
// the default flush interval (for GOSTATS_FLUSH_INTERVAL_SECONDS),
// rather than panicking - construction failing outright over a typo in
// this one opt-in knob is a worse outcome than silently not pruning.
func TestNewStoreToleratesOwnMalformedEnvVars(t *testing.T) {
	t.Run("PruneIdleSeconds", func(t *testing.T) {
		reset := testSetenv(t, "GOSTATS_PRUNE_IDLE_SECONDS", "not-an-int")
		defer reset()
		s := NewStore(nullSink{}, false).(*statStore)
		if s.pruneAfterFlushes != 0 {
			t.Errorf("pruneAfterFlushes = %d, want 0 (disabled on malformed input)", s.pruneAfterFlushes)
		}
	})

	t.Run("FlushIntervalSeconds", func(t *testing.T) {
		reset := testSetenv(t,
			"GOSTATS_PRUNE_IDLE_SECONDS", "10",
			"GOSTATS_FLUSH_INTERVAL_SECONDS", "not-an-int",
		)
		defer reset()
		s := NewStore(nullSink{}, false).(*statStore)
		// Falls back to DefaultFlushIntervalS (5): ceil(10/5) = 2.
		if s.pruneAfterFlushes != 2 {
			t.Errorf("pruneAfterFlushes = %d, want 2 (falls back to the default flush interval)", s.pruneAfterFlushes)
		}
	})
}

// A PruneIdleSeconds large enough to overflow uint32 after the
// seconds-to-flushes conversion must clamp, not wrap around - wrapping
// could turn "practically never prune" into "prune on every flush", the
// worst possible misreading of the operator's intent.
func TestNewStorePruneAfterFlushesClampsOnOverflow(t *testing.T) {
	reset := testSetenv(t,
		"GOSTATS_PRUNE_IDLE_SECONDS", "21474836485", // / 5 ceils to 4294967297, wraps to 1 if cast blindly
		"GOSTATS_FLUSH_INTERVAL_SECONDS", "5",
	)
	defer reset()
	s := NewStore(nullSink{}, false).(*statStore)
	if s.pruneAfterFlushes != math.MaxUint32 {
		t.Errorf("pruneAfterFlushes = %d, want %d (clamped, not wrapped)", s.pruneAfterFlushes, uint32(math.MaxUint32))
	}
}

// TestPrunedCounterIntermittentWriteNeverOrphans stresses the specific
// shape that exposed a permanent-orphan bug during development: a single
// held counter written intermittently against a store pruning as
// aggressively as possible (threshold=1). A continuously-hammered counter
// (see TestPrunedCounterRaceWithFlush) rarely goes idle, so it rarely
// reaches the prune branch at all; gaps between writes are what let the
// flusher's Range actually visit an idle-but-about-to-be-written-again
// counter, which is where the bug lived.
//
// The bug: rejoin() used to clear c.detached unconditionally, based on
// its LoadOrStore having reinserted c into the map. If the flusher
// deleted and re-detached c again in the gap between that LoadOrStore and
// the clear, the clear then landed after the second prune and left c
// outside the map with detached == 0 - a state maybeRejoin can never
// recover from, since it never calls rejoin() when detached reads 0.
// Every future write on that held reference was silently dropped forever.
//
// Fixed by never clearing detached from rejoin(): only Flush's own
// active branch does. See the comment on counter.rejoin.
//
// This covers a write racing Flush. A second, distinct orphan hazard -
// Flush racing itself, via two concurrent Flush() calls - is covered
// separately by TestConcurrentFlushesDoNotOrphanCounter; statStore.flushMu
// is what closes that one (see its comment for why a lock-free,
// generation-counter version of this same flag was tried and dropped).
func TestPrunedCounterIntermittentWriteNeverOrphans(t *testing.T) {
	sink := mock.NewSink()
	const threshold = 1
	s := &statStore{sink: sink, pruneAfterFlushes: threshold}

	c := s.NewCounter("intermittent")

	// A brief sleep between writes, not runtime.Gosched, is what actually
	// exposes the bug: it needs to be a real, if tiny, gap - long enough
	// for the tight-looping flusher to complete two full Flush cycles
	// inside it. The write's own Add() always leaves a pending delta the
	// very next flush drains via the active branch; only a *second*
	// flush within the same gap can find the counter idle again and
	// re-prune it. Verified empirically against the pre-fix code: this
	// shape lost a substantial fraction of increments in roughly 1 run
	// in 15, where a continuously-hammering multi-writer version (see
	// TestPrunedCounterRaceWithFlush) essentially never reached the
	// prune branch at all for the contended key.
	const writes = 300
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < writes; i++ {
			c.Inc()
			time.Sleep(50 * time.Microsecond)
		}
	}()

loop:
	for {
		select {
		case <-done:
			break loop
		default:
			s.Flush()
		}
	}

	// Deterministic drain: if c is currently detached, this Inc()
	// reattaches it (nothing else ever looks up this name), and the
	// following Flush reports the total - removing any dependence on
	// exactly when the last write landed relative to the last prune.
	c.Inc()
	s.Flush()

	want := uint64(writes + 1)
	if got, ok := sink.LoadCounter("intermittent"); !ok || got != want {
		t.Errorf("LoadCounter(%q) = (%d, %v), want (%d, true)", "intermittent", got, ok, want)
	}
}

// counter.latch was only ever called from the single Flush goroutine's
// Range before rejoin's forwarding path existed. That path calls
// c.latch() from whatever goroutine is writing to a permanently-detached,
// forwarding counter (see rejoin), so latch must now tolerate concurrent
// callers on the same object.
//
// The old implementation - value := c.Value(); lastSent :=
// atomic.SwapUint64(&c.lastSentValue, value); return value - lastSent -
// reads and swaps as two separate atomic operations. Nothing prevents a
// second caller's whole read-then-swap from completing in between this
// caller's read and its own swap. Forced deterministically (bypassing
// scheduling luck, which needs a very specific interleaving to land
// naturally): caller A reads currentValue=1, caller B reads
// currentValue=2 and swaps lastSentValue 0->2 (delta 2, correct so far),
// then A resumes and swaps lastSentValue 2->1 using its stale read -
// returning 1-2, which underflows to 18446744073709551615, and leaving
// lastSentValue at 1 even though the true reported value is 2. The next
// real latch() call then double-counts the 1-unit gap this created.
//
// This test pins the property the fix must have: a second caller whose
// read is already stale by the time it tries to commit must not be able
// to commit it - it must retry against fresh state instead.
func TestLatchDoesNotCommitStaleSwap(t *testing.T) {
	c := &counter{}
	atomic.StoreUint64(&c.currentValue, 1)

	// Caller B: reads later (sees more), commits first.
	atomic.StoreUint64(&c.currentValue, 2)
	if !atomic.CompareAndSwapUint64(&c.lastSentValue, 0, 2) {
		t.Fatal("setup: B's CAS should have succeeded against a fresh lastSentValue")
	}

	// Caller A: its read of currentValue=1 and lastSentValue=0 is now
	// stale (B already advanced lastSentValue to 2). Committing it
	// would regress lastSentValue backwards and corrupt the next delta.
	if atomic.CompareAndSwapUint64(&c.lastSentValue, 0, 1) {
		t.Fatal("A's stale attempt must not succeed against the CAS - it would regress lastSentValue")
	}
	if got := atomic.LoadUint64(&c.lastSentValue); got != 2 {
		t.Errorf("lastSentValue = %d, want 2 (must never regress once advanced)", got)
	}
}

// Concurrent writers plus concurrent latchers on the same counter, the
// shape rejoin's forwarding path creates when multiple goroutines write
// to a counter that's permanently detached and forwarding into another.
// Every increment must eventually be counted exactly once.
func TestLatchConcurrentCallersSumCorrect(t *testing.T) {
	c := &counter{}

	const writers = 4
	const perWriter = 500000
	var wg sync.WaitGroup
	wg.Add(writers)
	for g := 0; g < writers; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				c.Add(1)
			}
		}()
	}

	stop := make(chan struct{})
	const latchers = 4
	var sum uint64
	var lwg sync.WaitGroup
	lwg.Add(latchers)
	for g := 0; g < latchers; g++ {
		go func() {
			defer lwg.Done()
			var local uint64
			for {
				select {
				case <-stop:
					atomic.AddUint64(&sum, local)
					return
				default:
					local += c.latch()
				}
			}
		}()
	}
	wg.Wait()
	close(stop)
	lwg.Wait()
	atomic.AddUint64(&sum, c.latch()) // drain anything left after writers stop

	want := uint64(writers * perWriter)
	if sum != want {
		t.Errorf("sum of latch() = %d, want %d (diff %d)", sum, want, int64(sum)-int64(want))
	}
}

// Two concurrent Flush() calls can reopen the same class of orphan the
// rejoin fix closed, via the clear at the top of counters.Range: F1 sees
// a nonzero latch, flushes it, and is about to clear detached - based on
// having "just confirmed" via its own Range callback that c is in the
// map. That confirmation is stale by the time the clear actually
// executes if F2 (a second, fully independent Flush() call visiting the
// same key) deletes and re-detaches c in the gap. F1 then resumes and
// clobbers F2's detached=1 back to 0 while c sits outside the map -
// exactly the orphan state, just reached through Flush racing itself
// instead of a write racing Flush.
//
// Closed by statStore.flushMu, which serializes Flush() calls so F1 and
// F2 can no longer be "concurrent" in the sense this bug needs. Before
// that fix landed, this test failed reliably (see flushMu's comment for
// what was tried first and why it wasn't enough); it stays as a
// regression guard for this specific failure mode, deterministic now
// rather than probabilistic.
//
// This is realistic, not contrived: Store's own doc comment says "The
// store will flush either at the regular interval, or whenever Flush()
// is called" - a ticker-driven Start goroutine plus a manual Flush() call
// from elsewhere is exactly this shape.
func TestConcurrentFlushesDoNotOrphanCounter(t *testing.T) {
	sink := mock.NewSink()
	const threshold = 1
	s := &statStore{sink: sink, pruneAfterFlushes: threshold}

	c := s.NewCounter("racing-flushers")

	const writes = 300
	writesDone := make(chan struct{})
	go func() {
		defer close(writesDone)
		for i := 0; i < writes; i++ {
			c.Inc()
			time.Sleep(20 * time.Microsecond)
		}
	}()

	stop := make(chan struct{})
	var flushersWg sync.WaitGroup
	const flushers = 4
	flushersWg.Add(flushers)
	for i := 0; i < flushers; i++ {
		go func() {
			defer flushersWg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					s.Flush()
				}
			}
		}()
	}

	<-writesDone
	close(stop)
	flushersWg.Wait()

	// Deterministic drain: if c is currently detached, this Inc()
	// reattaches it (nothing else ever looks up this name), and the
	// following Flush reports the total.
	c.Inc()
	s.Flush()

	want := uint64(writes + 1)
	if got, ok := sink.LoadCounter("racing-flushers"); !ok || got != want {
		t.Errorf("LoadCounter(%q) = (%d, %v), want (%d, true)", "racing-flushers", got, ok, want)
	}
}

func BenchmarkStore_MutexContention(b *testing.B) {
	s := NewStore(nullSink{}, false)
	t := time.NewTicker(500 * time.Microsecond) // we want flush to contend with accessing metrics
	defer t.Stop()
	go s.Start(t)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bmID := strconv.Itoa(rand.Intn(1000))
		c := s.NewCounter(bmID)
		c.Inc()
		_ = c.Value()
	}
}

func BenchmarkStore_NewCounterWithTags(b *testing.B) {
	s := NewStore(nullSink{}, false)
	t := time.NewTicker(time.Hour) // don't flush
	defer t.Stop()
	go s.Start(t)
	tags := map[string]string{
		"tag1": "val1",
		"tag2": "val2",
		"tag3": "val3",
		"tag4": "val4",
		"tag5": "val5",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.NewCounterWithTags("counter_name", tags)
	}
}

func initBenchScope() (scope Scope, childTags map[string]string) {
	s := NewStore(nullSink{}, false)

	scopeTags := make(map[string]string, 5)
	childTags = make(map[string]string, 5)

	for i := 0; i < 5; i++ {
		tag := fmt.Sprintf("%dtag", i)
		val := fmt.Sprintf("%dval", i)
		scopeTags[tag] = val
		childTags["c"+tag] = "c" + val
	}

	scope = s.ScopeWithTags("scope", scopeTags)
	return
}

func BenchmarkStore_ScopeWithTags(b *testing.B) {
	scope, childTags := initBenchScope()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scope.NewCounterWithTags("counter_name", childTags)
	}
}

func BenchmarkStore_ScopeNoTags(b *testing.B) {
	scope, _ := initBenchScope()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scope.NewCounterWithTags("counter_name", nil)
	}
}

func BenchmarkParallelCounter(b *testing.B) {
	const N = 1000
	keys := make([]string, N)
	for i := 0; i < len(keys); i++ {
		keys[i] = randomString(b, 32)
	}

	s := NewStore(nullSink{}, false)
	t := time.NewTicker(time.Hour) // don't flush
	defer t.Stop()                 // never sends
	go s.Start(t)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		n := 0
		for pb.Next() {
			s.NewCounter(keys[n%N]).Inc()
		}
	})
}

func BenchmarkStoreNewPerInstanceCounter(b *testing.B) {
	b.Run("HasTag", func(b *testing.B) {
		var store statStore
		tags := map[string]string{
			"1":  "1",
			"2":  "2",
			"3":  "3",
			"_f": "xxx",
		}
		for i := 0; i < b.N; i++ {
			store.NewPerInstanceCounter("name", tags)
		}
	})

	b.Run("MissingTag", func(b *testing.B) {
		var store statStore
		tags := map[string]string{
			"1": "1",
			"2": "2",
			"3": "3",
			"4": "4",
		}
		for i := 0; i < b.N; i++ {
			store.NewPerInstanceCounter("name", tags)
		}
	})
}

// The pruning fields (idleFlushes, detached, store, name) grow counter from
// 16 to 48 bytes. That's real, so it's pinned here rather than left to
// drift unnoticed - fail if it grows further without a deliberate change.
// It's small against the ~130-180 bytes the sync.Map entry and key string
// already cost per stat (see BenchmarkStoreBytesPerStat).
func TestCounterTimerStructSizes(t *testing.T) {
	counterSize := unsafe.Sizeof(counter{})
	timerSize := unsafe.Sizeof(timer{})
	t.Logf("unsafe.Sizeof(counter{}) = %d bytes", counterSize)
	t.Logf("unsafe.Sizeof(timer{})   = %d bytes", timerSize)

	// Exact on 64-bit platforms, where every field's natural alignment
	// already lines up and there's no padding to vary; a loose bound
	// would let a future field addition slip through unnoticed. Loose on
	// other platforms since pointer/int width changes the layout and CI
	// only targets 64-bit.
	if strconv.IntSize == 64 {
		if counterSize != 48 {
			t.Errorf("unsafe.Sizeof(counter{}) = %d, want exactly 48 on a 64-bit platform", counterSize)
		}
		if timerSize != 48 {
			t.Errorf("unsafe.Sizeof(timer{}) = %d, want exactly 48 on a 64-bit platform", timerSize)
		}
		return
	}
	if counterSize > 64 {
		t.Errorf("unsafe.Sizeof(counter{}) = %d, want <= 64 (regression guard)", counterSize)
	}
	if timerSize > 64 {
		t.Errorf("unsafe.Sizeof(timer{}) = %d, want <= 64 (regression guard)", timerSize)
	}
}

// BenchmarkStoreBytesPerStat reports the total heap cost of tracking N
// unique counters: the sync.Map entry and key string every counter always
// paid, plus the counter struct itself. It does NOT split by pruning
// enabled/disabled - unsafe.Sizeof(counter{}) is a compile-time struct
// layout, so every counter carries the same 48 bytes whether or not the
// store's pruneAfterFlushes is nonzero; PruningEnabled would just report
// the same number with extra noise. TestCounterTimerStructSizes is the
// right place to see that this feature's fields cost 16 -> 48 bytes;
// this benchmark exists to show that delta is small next to the total.
//
// Run with -benchtime=10x for a quick, low-noise read; the default
// adaptive N re-runs the whole N-counter build repeatedly and mostly just
// burns time once the metric has stabilized.
func BenchmarkStoreBytesPerStat(b *testing.B) {
	const n = 20000
	for i := 0; i < b.N; i++ {
		s := &statStore{sink: nullSink{}}
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for j := 0; j < n; j++ {
			s.NewCounter("counter_" + strconv.Itoa(j))
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/float64(n), "bytes/counter")
		runtime.KeepAlive(s)
	}
}

// BenchmarkCardinalityFlood is the number for the ticket: it simulates the
// reported failure mode - a flood of tag combinations each written once
// and never again, interspersed with flushes - and reports how many
// counters remain live at the end. Without pruning this equals the flood
// size (unbounded growth); with pruning it stays small regardless of the
// flood size, since one-shot entries age out a few flushes after their
// only write.
//
// The flood size is fixed rather than scaled by b.N so the benchmark's
// cost doesn't balloon under the default adaptive iteration count; run
// with -benchtime=1x for a single clean measurement.
func BenchmarkCardinalityFlood(b *testing.B) {
	const floodSize = 5000
	for _, tc := range []struct {
		name  string
		prune uint32
	}{
		{"PruningDisabled", 0},
		{"PruningEnabled", 4},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				s := &statStore{sink: nullSink{}, pruneAfterFlushes: tc.prune}
				for j := 0; j < floodSize; j++ {
					s.NewCounter("flood_" + strconv.Itoa(j)).Inc()
					if j%10 == 0 {
						s.Flush()
					}
				}
				// Let anything still idle age out, mirroring how the map
				// settles between bursts in production.
				for k := uint32(0); k < tc.prune+1; k++ {
					s.Flush()
				}
				b.ReportMetric(float64(mapLen(&s.counters)), "live-counters")
			}
		})
	}
}

// BenchmarkCounterAdd and BenchmarkCounterInc are the direct rebuttal to
// PR #158's +3443% regression on NewCounter (a global-mutex LRU
// reordering on every access): with pruning disabled, store is nil, so
// the added check is a single non-atomic pointer comparison - the hot
// path is unchanged from before this feature existed. With pruning
// enabled, it costs exactly one additional atomic load.
func BenchmarkCounterAdd(b *testing.B) {
	b.Run("PruningDisabled", func(b *testing.B) {
		c := &counter{}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Add(1)
		}
	})
	b.Run("PruningEnabled", func(b *testing.B) {
		s := &statStore{sink: nullSink{}, pruneAfterFlushes: 4}
		c := s.newCounter("bench_counter")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Add(1)
		}
	})
}

func BenchmarkCounterInc(b *testing.B) {
	b.Run("PruningDisabled", func(b *testing.B) {
		c := &counter{}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Inc()
		}
	})
	b.Run("PruningEnabled", func(b *testing.B) {
		s := &statStore{sink: nullSink{}, pruneAfterFlushes: 4}
		c := s.newCounter("bench_counter")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Inc()
		}
	})
}

// BenchmarkStoreNewCounterParallel measures lookup contention under
// concurrent access to a fixed set of counters - the scenario PR #158's
// global-mutex LRU cache regressed.
func BenchmarkStoreNewCounterParallel(b *testing.B) {
	s := NewStore(nullSink{}, false)
	tick := time.NewTicker(time.Hour) // don't flush
	defer tick.Stop()
	go s.Start(tick)
	names := new([2048]string)
	for i := 0; i < len(names); i++ {
		names[i] = "counter_" + strconv.Itoa(i)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for i := 0; pb.Next(); i++ {
			s.NewCounter(names[i%len(names)])
		}
	})
}

// BenchmarkStoreFlush measures Flush's cost, with pruning disabled vs
// enabled, over N counters and N timers that are written on every
// iteration so none of them ever go idle. The added idle-tracking and
// prune logic runs on every entry on every flush regardless, so this is
// where a regression on the common (nothing to prune) path would show.
//
// Every entry must stay active: with pruning enabled, anything that goes
// idle for pruneAfterFlushes iterations is deleted, and once the whole
// map empties out the benchmark degenerates into measuring Flush() on an
// empty store - which is exactly the bug an earlier version of this
// benchmark had (it wrote to each entry once during setup instead of on
// every iteration, so pruning deleted everything within the first few
// iterations and the reported cost was ~1000x too fast).
//
// The writes that keep entries active happen with the timer stopped, so
// only Flush's own cost is measured.
func BenchmarkStoreFlush(b *testing.B) {
	const n = 2048
	for _, tc := range []struct {
		name  string
		prune uint32
	}{
		{"PruningDisabled", 0},
		{"PruningEnabled", 4},
	} {
		b.Run(tc.name, func(b *testing.B) {
			s := &statStore{sink: nullSink{}, pruneAfterFlushes: tc.prune}
			counters := make([]Counter, n)
			timers := make([]Timer, n)
			for i := 0; i < n; i++ {
				id := strconv.Itoa(i)
				counters[i] = s.NewCounter("counter_" + id)
				timers[i] = s.NewTimer("timer_" + id)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				for j := 0; j < n; j++ {
					counters[j].Inc()
					timers[j].AddValue(1)
				}
				b.StartTimer()
				s.Flush()
			}
		})
	}
}
