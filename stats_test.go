package stats

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
