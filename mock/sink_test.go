package mock

import (
	"math"
	"runtime"
	"sync"
	"testing"
)

func TestFlushTimer(t *testing.T) {
	sink := NewSink()
	var exp float64
	for i := 0; i < 10000; i++ {
		sink.FlushTimer("timer", 1)
		exp++
	}
	sink.AssertTimerEquals(t, "timer", exp)

	// test limits

	sink.Reset()
	sink.FlushTimer("timer", math.MaxFloat64)
	sink.AssertTimerEquals(t, "timer", math.MaxFloat64)

	sink.Reset()
	sink.FlushTimer("timer", math.SmallestNonzeroFloat64)
	sink.AssertTimerEquals(t, "timer", math.SmallestNonzeroFloat64)
}

func TestThreadSafeSinkReset(t *testing.T) {
	const N = 2000
	sink := NewSink()
	funcs := [...]func(){
		func() { sink.FlushCounter("name", 1) },
		func() { sink.FlushGauge("name", 1) },
		func() { sink.FlushTimer("name", 1) },
		func() { sink.LoadCounter("name") },
		func() { sink.LoadGauge("name") },
		func() { sink.LoadTimer("name") },
		func() { sink.Counter("name") },
		func() { sink.Gauge("name") },
		func() { sink.Timer("name") },
		func() { sink.CounterCallCount("name") },
		func() { sink.GaugeCallCount("name") },
		func() { sink.TimerCallCount("name") },
	}
	numCPU := runtime.NumCPU() - 1
	if numCPU < 2 {
		numCPU = 2
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				sink.Reset()
			}
		}
	}()
	var wg sync.WaitGroup
	for i := 0; i < numCPU; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < N; i++ {
				funcs[i%len(funcs)]()
			}
		}()
	}
	wg.Wait()
	close(done)
}
