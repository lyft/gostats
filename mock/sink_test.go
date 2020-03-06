package mock

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSink(t *testing.T) {
	testCounter := func(t *testing.T, exp uint64, sink *Sink) {
		t.Helper()
		const name = "test-counter"
		sink.FlushCounter(name, exp)
		sink.AssertCounterEquals(t, name, exp)
		sink.AssertCounterExists(t, name)
		sink.AssertCounterCallCount(t, name, 1)
		if n := sink.Counter(name); n != exp {
			t.Errorf("Counter(): want: %d got: %d", exp, n)
		}
	}
	testGauge := func(t *testing.T, exp uint64, sink *Sink) {
		const name = "test-gauge"
		sink.FlushGauge(name, exp)
		sink.AssertGaugeEquals(t, name, exp)
		sink.AssertGaugeExists(t, name)
		sink.AssertGaugeCallCount(t, name, 1)
		if n := sink.Gauge(name); n != exp {
			t.Errorf("Gauge(): want: %d got: %d", exp, n)
		}
	}
	testTimer := func(t *testing.T, exp float64, sink *Sink) {
		const name = "test-timer"
		sink.FlushTimer(name, exp)
		sink.AssertTimerEquals(t, name, exp)
		sink.AssertTimerExists(t, name)
		sink.AssertTimerCallCount(t, name, 1)
		if n := sink.Timer(name); n != exp {
			t.Errorf("Timer(): want: %f got: %f", exp, n)
		}
	}
	// test 0..1 - we want to make sure that 0 still registers a stat
	for i := 0; i < 2; i++ {
		t.Run("Counter", func(t *testing.T) {
			testCounter(t, uint64(i), NewSink())
		})
		t.Run("Gauge", func(t *testing.T) {
			testGauge(t, uint64(i), NewSink())
		})
		t.Run("Timer", func(t *testing.T) {
			testTimer(t, float64(i), NewSink())
		})
		// all together now
		sink := NewSink()
		testCounter(t, 1, sink)
		testGauge(t, 1, sink)
		testTimer(t, 1, sink)
	}
}

// Test that the zero Sink is ready for use.
func TestSinkLazyInit(t *testing.T) {
	var s Sink
	s.FlushCounter("counter", 1)
	s.AssertCounterEquals(t, "counter", 1)
}

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

func TestSink_ThreadSafe(t *testing.T) {
	const N = 2000
	sink := NewSink()
	var (
		counterCalls = new(int64)
		gaugeCalls   = new(int64)
		timerCalls   = new(int64)
		counterVal   = new(uint64)
		gaugeVal     = new(uint64)
	)
	funcs := [...]func(){
		func() {
			atomic.AddInt64(counterCalls, 1)
			atomic.AddUint64(counterVal, 1)
			sink.FlushCounter("name", 1)
		},
		func() {
			atomic.AddInt64(gaugeCalls, 1)
			atomic.AddUint64(gaugeVal, 1)
			sink.FlushGauge("name", 1)
		},
		func() {
			atomic.AddInt64(timerCalls, 1)
			sink.FlushTimer("name", 1)
		},
	}
	numCPU := runtime.NumCPU()
	if numCPU < 2 {
		numCPU = 2
	}
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

	sink.AssertCounterCallCount(t, "name", int(atomic.LoadInt64(counterCalls)))
	sink.AssertGaugeCallCount(t, "name", int(atomic.LoadInt64(gaugeCalls)))
	sink.AssertTimerCallCount(t, "name", int(atomic.LoadInt64(timerCalls)))

	sink.AssertCounterEquals(t, "name", atomic.LoadUint64(counterVal))
	sink.AssertGaugeEquals(t, "name", atomic.LoadUint64(gaugeVal))
}

func TestSink_ThreadSafe_Reset(t *testing.T) {
	const N = 2000
	sink := NewSink()
	funcs := [...]func(){
		func() { sink.Flush() },
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

// TestFatalExample is an example usage of Fatal()
func TestFatalExample(t *testing.T) {
	sink := NewSink()
	sink.FlushCounter("name", 1)
	sink.AssertCounterEquals(Fatal(t), "name", 1)
}

func setupBenchmark(prefix string) (*Sink, [128]string) {
	var names [128]string
	if prefix == "" {
		prefix = "mock_sink"
	}
	for i := 0; i < len(names); i++ {
		names[i] = fmt.Sprintf("%s_%d", prefix, i)
	}
	sink := NewSink()
	return sink, names
}

func BenchmarkFlushCounter(b *testing.B) {
	sink, names := setupBenchmark("counter")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink.FlushCounter(names[i%len(names)], uint64(i))
	}
}

func BenchmarkFlushCounter_Parallel(b *testing.B) {
	sink, names := setupBenchmark("counter")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for i := 0; pb.Next(); i++ {
			sink.FlushCounter(names[i%len(names)], uint64(i))
		}
	})
}

func BenchmarkFlushTimer(b *testing.B) {
	const f = 1234.5678
	sink, names := setupBenchmark("timer")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink.FlushTimer(names[i%len(names)], f)
	}
}

func BenchmarkFlushTimer_Parallel(b *testing.B) {
	const f = 1234.5678
	sink, names := setupBenchmark("timer")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for i := 0; pb.Next(); i++ {
			sink.FlushTimer(names[i%len(names)], f)
		}
	})
}
