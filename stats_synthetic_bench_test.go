package stats

import "testing"

// syntheticTrace simulates the call pattern of a service (e.g. supplycost)
// that holds a long-lived root Scope but re-derives nested scopes and
// tagged metric handles afresh, inline, on every request - rather than
// caching subScope/Counter/Gauge/Timer references itself. This is the exact
// pattern that made gostats' own joinScopes/MergeTags/Serialize path show up
// as a large fraction of allocations under profiling: every call below is,
// from the caller's perspective, "the same metric" as the last call, but
// prior to caching gostats had no way to know that without redoing the full
// key computation from scratch each time.
func syntheticTrace(root Scope) {
	graph := root.ScopeWithTags("graph", map[string]string{"graph_name": "supply_estimate"})
	node := graph.ScopeWithTags("node", map[string]string{"source_node": "waypoint_bonus"})

	regionTags := map[string]string{"region_code": "us-east-1", "ride_type": "standard"}
	node.NewCounterWithTags("attempt", regionTags).Inc()
	node.NewCounterWithTags("success", regionTags).Inc()
	node.NewGaugeWithTags("queue_depth", map[string]string{"region_code": "us-east-1"}).Set(5)
	node.NewTimerWithTags("latency", map[string]string{"region_code": "us-east-1"}).AddValue(1500)

	fee := root.ScopeWithTags("fee", map[string]string{"fee_type": "toll"}).
		ScopeWithTags("region", map[string]string{"region_code": "us-east-1"})
	fee.NewCounterWithTags("charged", map[string]string{"toll_period": "peak"}).Inc()
	fee.NewCounterWithTags("waived", map[string]string{"toll_period": "peak"}).Inc()

	// A per-instance metric, deliberately left out of the memoized fast path
	// (see stats.go) - included here so the benchmark reflects the mixed
	// workload real traces have, not just the calls that got faster.
	node.NewPerInstanceCounter("dispatch", map[string]string{"region_code": "us-east-1"}).Inc()
}

// BenchmarkSyntheticTrace runs syntheticTrace repeatedly against a single,
// long-lived root Scope - the steady-state condition once a service has
// warmed up and every (name, tags) combination below has already been seen
// at least once. Compare -benchmem output against the pre-memoization
// revision to see the allocation delta from caching Scope/Counter/Gauge/
// Timer lookups (see the "Memoize tag-based Scope/Counter/Gauge/Timer
// lookups" commit).
func BenchmarkSyntheticTrace(b *testing.B) {
	store := NewStore(NewNullSink(), false)
	root := store.Scope("supplycost")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		syntheticTrace(root)
	}
}

// BenchmarkSyntheticTraceParallel is the same trace under concurrent load
// (multiple goroutines sharing the same root Scope), which is the realistic
// shape for a request-serving service.
func BenchmarkSyntheticTraceParallel(b *testing.B) {
	store := NewStore(NewNullSink(), false)
	root := store.Scope("supplycost")

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			syntheticTrace(root)
		}
	})
}
