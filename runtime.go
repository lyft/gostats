package stats

import (
	"runtime"
)

type runtimeStats struct {
	alloc      Gauge   // bytes allocated and not yet freed
	totalAlloc Counter // bytes allocated (even if freed)
	sys        Gauge   // bytes obtained from system (sum of XxxSys below)
	lookups    Counter // number of pointer lookups
	mallocs    Counter // number of mallocs
	frees      Counter // number of frees

	// Main allocation heap statistics
	heapAlloc    Gauge // bytes allocated and not yet freed (same as Alloc above)
	heapSys      Gauge // bytes obtained from system
	heapIdle     Gauge // bytes in idle spans
	heapInuse    Gauge // bytes in non-idle span
	heapReleased Gauge // bytes released to the OS
	heapObjects  Gauge // total number of allocated objects

	// Garbage collector statistics.
	nextGC       Gauge // next collection will happen when HeapAlloc ≥ this amount
	lastGC       Gauge // end time of last collection (nanoseconds since 1970)
	pauseTotalNs Counter
	numGC        Counter
	gcCPUPercent Gauge

	numGoroutine Gauge
}

func NewRuntimeStats(scope Scope) StatGenerator {
	ret := runtimeStats{}

	ret.alloc = scope.NewGauge("alloc")
	ret.totalAlloc = scope.NewCounter("totalAlloc")
	ret.sys = scope.NewGauge("sys")
	ret.lookups = scope.NewCounter("lookups")
	ret.mallocs = scope.NewCounter("mallocs")
	ret.frees = scope.NewCounter("frees")

	ret.heapAlloc = scope.NewGauge("heapAlloc")
	ret.heapSys = scope.NewGauge("heapSys")
	ret.heapIdle = scope.NewGauge("heapIdle")
	ret.heapInuse = scope.NewGauge("heapInuse")
	ret.heapReleased = scope.NewGauge("heapReleased")
	ret.heapObjects = scope.NewGauge("heapObjects")

	ret.nextGC = scope.NewGauge("nextGC")
	ret.lastGC = scope.NewGauge("lastGC")
	ret.pauseTotalNs = scope.NewCounter("pauseTotalNs")
	ret.numGC = scope.NewCounter("numGC")
	ret.gcCPUPercent = scope.NewGauge("gcCPUPercent")

	ret.numGoroutine = scope.NewGauge("numGoroutine")

	return ret
}

func (r runtimeStats) GenerateStats() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	r.alloc.Set(memStats.Alloc)
	r.totalAlloc.Set(memStats.TotalAlloc)
	r.mallocs.Set(memStats.Mallocs)
	r.frees.Set(memStats.Frees)

	r.heapAlloc.Set(memStats.HeapAlloc)
	r.heapSys.Set(memStats.HeapSys)
	r.heapIdle.Set(memStats.HeapIdle)
	r.heapInuse.Set(memStats.HeapInuse)
	r.heapReleased.Set(memStats.HeapReleased)
	r.heapObjects.Set(memStats.HeapObjects)

	r.nextGC.Set(memStats.NextGC)
	r.lastGC.Set(memStats.LastGC)
	r.pauseTotalNs.Set(memStats.PauseTotalNs)
	r.numGC.Set(uint64(memStats.NumGC))
	r.gcCPUPercent.Set(uint64(memStats.GCCPUFraction * 100))

	r.numGoroutine.Set(uint64(runtime.NumGoroutine()))
}
