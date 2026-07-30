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
//
// NewPerInstanceCounter is deliberately excluded here (see
// BenchmarkSyntheticTracePerInstance) - it isn't memoized (see stats.go), so
// mixing it in would stop this benchmark from showing the true 0-allocs/op
// steady state of the calls that are.
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

// pooledTags holds one map[string]string per distinct tag set used by
// syntheticTracePooled, built once and reused across every call instead of
// allocated fresh inline. gostats never mutates a tags map handed to it, so
// sharing these across goroutines (BenchmarkSyntheticTracePooledParallel) is
// safe.
type pooledTags struct {
	graph, node, region, queueDepth, latency, fee, feeRegion, toll, dispatch map[string]string
}

func newPooledTags() *pooledTags {
	return &pooledTags{
		graph:      map[string]string{"graph_name": "supply_estimate"},
		node:       map[string]string{"source_node": "waypoint_bonus"},
		region:     map[string]string{"region_code": "us-east-1", "ride_type": "standard"},
		queueDepth: map[string]string{"region_code": "us-east-1"},
		latency:    map[string]string{"region_code": "us-east-1"},
		fee:        map[string]string{"fee_type": "toll"},
		feeRegion:  map[string]string{"region_code": "us-east-1"},
		toll:       map[string]string{"toll_period": "peak"},
		dispatch:   map[string]string{"region_code": "us-east-1"},
	}
}

// syntheticTracePooled is syntheticTrace with every tags map built once (via
// newPooledTags) and reused across calls, as a caller might do by holding a
// package-level or per-instance pool of tag maps instead of building a fresh
// map[string]string literal on every call. This removes the caller-side map
// allocations from the measurement, isolating gostats' own internal
// allocations (the joinScopes/MergeTags/Serialize path and the scopeCache
// bookkeeping around it) so the -benchmem delta between the pre- and
// post-memoization revisions reflects gostats' contribution alone.
//
// Like syntheticTrace, NewPerInstanceCounter is excluded - see
// BenchmarkSyntheticTracePerInstancePooled.
func syntheticTracePooled(root Scope, p *pooledTags) {
	graph := root.ScopeWithTags("graph", p.graph)
	node := graph.ScopeWithTags("node", p.node)

	node.NewCounterWithTags("attempt", p.region).Inc()
	node.NewCounterWithTags("success", p.region).Inc()
	node.NewGaugeWithTags("queue_depth", p.queueDepth).Set(5)
	node.NewTimerWithTags("latency", p.latency).AddValue(1500)

	fee := root.ScopeWithTags("fee", p.fee).ScopeWithTags("region", p.feeRegion)
	fee.NewCounterWithTags("charged", p.toll).Inc()
	fee.NewCounterWithTags("waived", p.toll).Inc()
}

// BenchmarkSyntheticTracePooled is BenchmarkSyntheticTrace with pooled tag
// maps (see syntheticTracePooled) - the allocs/op and B/op it reports are
// gostats' own internal cost, with the caller-side map-literal allocations
// that BenchmarkSyntheticTrace also pays for removed from the measurement.
func BenchmarkSyntheticTracePooled(b *testing.B) {
	store := NewStore(NewNullSink(), false)
	root := store.Scope("supplycost")
	tags := newPooledTags()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		syntheticTracePooled(root, tags)
	}
}

// BenchmarkSyntheticTracePooledParallel is BenchmarkSyntheticTracePooled
// under concurrent load, sharing both the root Scope and the pooled tags
// across goroutines.
func BenchmarkSyntheticTracePooledParallel(b *testing.B) {
	store := NewStore(NewNullSink(), false)
	root := store.Scope("supplycost")
	tags := newPooledTags()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			syntheticTracePooled(root, tags)
		}
	})
}

// BenchmarkSyntheticTracePerInstance measures NewPerInstanceCounter alone,
// with a fresh tags map[string]string literal built inline each call (as
// syntheticTrace does for its other calls). NewPerInstanceCounter is not
// memoized by scopeCache (see stats.go), so this should show no allocation
// improvement over the pre-memoization revision - it's a control, not a
// regression.
func BenchmarkSyntheticTracePerInstance(b *testing.B) {
	store := NewStore(NewNullSink(), false)
	node := store.Scope("supplycost").ScopeWithTags("graph", map[string]string{"graph_name": "supply_estimate"}).
		ScopeWithTags("node", map[string]string{"source_node": "waypoint_bonus"})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.NewPerInstanceCounter("dispatch", map[string]string{"region_code": "us-east-1"}).Inc()
	}
}

// BenchmarkSyntheticTracePerInstancePooled is
// BenchmarkSyntheticTracePerInstance with the tags map pooled (see
// pooledTags), isolating NewPerInstanceCounter's own internal allocations
// from the caller-side map-literal cost.
func BenchmarkSyntheticTracePerInstancePooled(b *testing.B) {
	store := NewStore(NewNullSink(), false)
	node := store.Scope("supplycost").ScopeWithTags("graph", map[string]string{"graph_name": "supply_estimate"}).
		ScopeWithTags("node", map[string]string{"source_node": "waypoint_bonus"})
	tags := newPooledTags()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.NewPerInstanceCounter("dispatch", tags.dispatch).Inc()
	}
}
