package stats

import (
	"sync"
	"testing"
)

// Ensure repeated calls with equal, but distinct, tags maps return the exact
// same Counter/Gauge/Timer/Scope, and that the value recorded through a
// cache hit is identical to going through the slow path directly.

func TestSubScopeCounterCacheHit(t *testing.T) {
	store := NewStore(&testStatSink{}, false)
	scope := store.Scope("app")

	tags1 := map[string]string{"region_code": "us-east-1", "ride_type": "standard"}
	tags2 := map[string]string{"ride_type": "standard", "region_code": "us-east-1"} // different map, same content

	c1 := scope.NewCounterWithTags("attempt", tags1)
	c2 := scope.NewCounterWithTags("attempt", tags2)
	if c1 != c2 {
		t.Fatal("expected the same Counter for equal tags regardless of map identity/order")
	}

	c1.Add(2)
	c2.Add(3) // should land on the same counter
	if v := c1.(*counter).String(); v != "5" {
		t.Fatalf("got: %s want: 5", v)
	}
}

func TestSubScopeCounterCacheDifferentTags(t *testing.T) {
	store := NewStore(&testStatSink{}, false)
	scope := store.Scope("app")

	c1 := scope.NewCounterWithTags("attempt", map[string]string{"region_code": "us-east-1"})
	c2 := scope.NewCounterWithTags("attempt", map[string]string{"region_code": "us-west-2"})
	if c1 == c2 {
		t.Fatal("different tag values must not collide in the cache")
	}
}

func TestSubScopeGaugeAndTimerCacheHit(t *testing.T) {
	store := NewStore(&testStatSink{}, false)
	scope := store.Scope("app")

	tags := map[string]string{"graph_name": "supply_estimate"}
	g1 := scope.NewGaugeWithTags("queue_depth", tags)
	g2 := scope.NewGaugeWithTags("queue_depth", map[string]string{"graph_name": "supply_estimate"})
	if g1 != g2 {
		t.Fatal("expected the same Gauge for equal tags")
	}

	tm1 := scope.NewTimerWithTags("latency", tags)
	tm2 := scope.NewTimerWithTags("latency", map[string]string{"graph_name": "supply_estimate"})
	if tm1 != tm2 {
		t.Fatal("expected the same Timer for equal tags")
	}

	mt1 := scope.NewMilliTimerWithTags("latency_ms", tags)
	mt2 := scope.NewMilliTimerWithTags("latency_ms", map[string]string{"graph_name": "supply_estimate"})
	if mt1 != mt2 {
		t.Fatal("expected the same milli Timer for equal tags")
	}
}

func TestSubScopeScopeCacheHit(t *testing.T) {
	store := NewStore(&testStatSink{}, false)
	scope := store.Scope("app")

	child1 := scope.Scope("nested")
	child2 := scope.Scope("nested")
	if child1 != child2 {
		t.Fatal("expected the same child Scope for repeated Scope() calls with the same name")
	}

	tagged1 := scope.ScopeWithTags("waypoint_bonus", map[string]string{"graph_name": "supply_estimate"})
	tagged2 := scope.ScopeWithTags("waypoint_bonus", map[string]string{"graph_name": "supply_estimate"})
	if tagged1 != tagged2 {
		t.Fatal("expected the same child Scope for repeated ScopeWithTags() calls with equal tags")
	}

	// A cached child scope must itself still be a working, cached Scope.
	c1 := child1.NewCounter("hits")
	c2 := child2.NewCounter("hits")
	if c1 != c2 {
		t.Fatal("counters created via a cached child scope should also be cached/identical")
	}
}

func TestStoreRootCounterCacheHit(t *testing.T) {
	store := NewStore(&testStatSink{}, false)

	tags := map[string]string{"region_code": "us-east-1"}
	c1 := store.NewCounterWithTags("attempt", tags)
	c2 := store.NewCounterWithTags("attempt", map[string]string{"region_code": "us-east-1"})
	if c1 != c2 {
		t.Fatal("expected the same Counter for equal tags at the store root")
	}
}

func TestStoreRootScopeCacheHit(t *testing.T) {
	store := NewStore(&testStatSink{}, false)

	s1 := store.Scope("app")
	s2 := store.Scope("app")
	if s1 != s2 {
		t.Fatal("expected the same root child Scope for repeated Scope() calls with the same name")
	}
}

// Regression check: caching must not change what actually gets flushed.
func TestScopeCacheFlushOutputUnchanged(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, false)
	scope := store.Scope("app").ScopeWithTags("child", map[string]string{"k": "v"})

	scope.NewCounterWithTags("attempt", map[string]string{"region_code": "us-east-1"}).Add(1)
	scope.NewCounterWithTags("attempt", map[string]string{"region_code": "us-east-1"}).Add(1) // cache hit
	store.Flush()

	const want = "app.child.attempt.__k=v.__region_code=us-east-1:2|c\n"
	if sink.record != want {
		t.Fatalf("got: %q want: %q", sink.record, want)
	}
}

// Concurrent callers hitting the same cache entries must not race and must
// converge on a single created instance (run with -race).
func TestScopeCacheConcurrentSafety(t *testing.T) {
	store := NewStore(&testStatSink{}, false)
	scope := store.Scope("app")

	const n = 64
	results := make([]Counter, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = scope.NewCounterWithTags("attempt", map[string]string{"region_code": "us-east-1"})
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 1; i < n; i++ {
		if results[i] != results[0] {
			t.Fatalf("expected all concurrent callers to converge on the same Counter, index %d differed", i)
		}
	}
}
