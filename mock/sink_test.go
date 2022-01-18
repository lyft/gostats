package mock_test

import (
	"fmt"
	"math"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/lyft/gostats/mock"
)

type ErrorTest struct {
	testing.TB
	errMsg string
}

func NewErrorTest(t testing.TB) *ErrorTest {
	return &ErrorTest{TB: t}
}

func (t *ErrorTest) Error(args ...interface{}) {
	t.errMsg = fmt.Sprint(args...)
}

func (t *ErrorTest) Errorf(format string, args ...interface{}) {
	t.errMsg = fmt.Sprintf(format, args...)
}

func (t *ErrorTest) AssertErrorMsg(format string, args ...interface{}) {
	exp := fmt.Sprintf(format, args...)
	if t.errMsg != exp {
		t.TB.Errorf("Expected error message: `%s` got: `%s`", exp, t.errMsg)
	}
}

func AssertErrorMsg(t testing.TB, fn func(t testing.TB), format string, args ...interface{}) {
	t.Helper()
	x := NewErrorTest(t)
	fn(x)
	x.AssertErrorMsg(format, args...)
}

func (t *ErrorTest) Reset() testing.TB {
	t.errMsg = ""
	return t
}

func TestSink(t *testing.T) {
	testCounter := func(t *testing.T, exp uint64, sink *mock.Sink) {
		t.Helper()
		const name = "test-counter"
		sink.FlushCounter(name, exp)
		sink.AssertCounterExists(t, name)
		sink.AssertCounterEquals(t, name, exp)
		sink.AssertCounterCallCount(t, name, 1)
		if n := sink.Counter(name); n != exp {
			t.Errorf("Counter(): want: %d got: %d", exp, n)
		}

		const missing = name + "-MISSING"
		sink.AssertCounterNotExists(t, missing)

		fns := []func(t testing.TB){
			func(t testing.TB) { sink.AssertCounterExists(t, missing) },
			func(t testing.TB) { sink.AssertCounterEquals(t, missing, 9999) },
			func(t testing.TB) { sink.AssertCounterCallCount(t, missing, 9999) },
		}
		for _, fn := range fns {
			AssertErrorMsg(t, fn, "gostats/mock: Counter (%q): not found in: [\"test-counter\"]", missing)
		}

		AssertErrorMsg(t, func(t testing.TB) {
			sink.AssertCounterEquals(t, name, 9999)
		}, "gostats/mock: Counter (%q): Expected: %d Got: %d", name, 9999, exp)

		AssertErrorMsg(t, func(t testing.TB) {
			sink.AssertCounterCallCount(t, name, 9999)
		}, "gostats/mock: Counter (%q) Call Count: Expected: %d Got: %d", name, 9999, 1)

		AssertErrorMsg(t, func(t testing.TB) {
			sink.AssertCounterNotExists(t, name)
		}, "gostats/mock: Counter (%q): expected Counter to not exist", name)
	}

	testGauge := func(t *testing.T, exp uint64, sink *mock.Sink) {
		const name = "test-gauge"
		sink.FlushGauge(name, exp)
		sink.AssertGaugeExists(t, name)
		sink.AssertGaugeEquals(t, name, exp)
		sink.AssertGaugeCallCount(t, name, 1)
		if n := sink.Gauge(name); n != exp {
			t.Errorf("Gauge(): want: %d got: %d", exp, n)
		}

		const missing = name + "-MISSING"
		sink.AssertGaugeNotExists(t, missing)

		fns := []func(t testing.TB){
			func(t testing.TB) { sink.AssertGaugeExists(t, missing) },
			func(t testing.TB) { sink.AssertGaugeEquals(t, missing, 9999) },
			func(t testing.TB) { sink.AssertGaugeCallCount(t, missing, 9999) },
		}
		for _, fn := range fns {
			AssertErrorMsg(t, fn, "gostats/mock: Gauge (%q): not found in: [\"test-gauge\"]", missing)
		}

		AssertErrorMsg(t, func(t testing.TB) {
			sink.AssertGaugeEquals(t, name, 9999)
		}, "gostats/mock: Gauge (%q): Expected: %d Got: %d", name, 9999, exp)

		AssertErrorMsg(t, func(t testing.TB) {
			sink.AssertGaugeCallCount(t, name, 9999)
		}, "gostats/mock: Gauge (%q) Call Count: Expected: %d Got: %d", name, 9999, 1)

		AssertErrorMsg(t, func(t testing.TB) {
			sink.AssertGaugeNotExists(t, name)
		}, "gostats/mock: Gauge (%q): expected Gauge to not exist", name)
	}

	testTimer := func(t *testing.T, exp float64, sink *mock.Sink) {
		const name = "test-timer"
		sink.FlushTimer(name, exp)
		sink.AssertTimerExists(t, name)
		sink.AssertTimerEquals(t, name, exp)
		sink.AssertTimerCallCount(t, name, 1)
		if n := sink.Timer(name); n != exp {
			t.Errorf("Timer(): want: %f got: %f", exp, n)
		}

		const missing = name + "-MISSING"
		sink.AssertTimerNotExists(t, missing)

		fns := []func(t testing.TB){
			func(t testing.TB) { sink.AssertTimerExists(t, missing) },
			func(t testing.TB) { sink.AssertTimerEquals(t, missing, 9999) },
			func(t testing.TB) { sink.AssertTimerCallCount(t, missing, 9999) },
		}
		for _, fn := range fns {
			AssertErrorMsg(t, fn, "gostats/mock: Timer (%q): not found in: [\"test-timer\"]", missing)
		}

		AssertErrorMsg(t, func(t testing.TB) {
			sink.AssertTimerEquals(t, name, 9999)
		}, "gostats/mock: Timer (%q): Expected: %f Got: %f", name, 9999.0, exp)

		AssertErrorMsg(t, func(t testing.TB) {
			sink.AssertTimerCallCount(t, name, 9999)
		}, "gostats/mock: Timer (%q) Call Count: Expected: %d Got: %d", name, 9999, 1)

		AssertErrorMsg(t, func(t testing.TB) {
			sink.AssertTimerNotExists(t, name)
		}, "gostats/mock: Timer (%q): expected Timer to not exist", name)
	}

	// test 0..1 - we want to make sure that 0 still registers a stat
	for i := 0; i < 2; i++ {
		t.Run("Counter", func(t *testing.T) {
			testCounter(t, uint64(i), mock.NewSink())
		})
		t.Run("Gauge", func(t *testing.T) {
			testGauge(t, uint64(i), mock.NewSink())
		})
		t.Run("Timer", func(t *testing.T) {
			testTimer(t, float64(i), mock.NewSink())
		})
		// all together now
		sink := mock.NewSink()
		testCounter(t, 1, sink)
		testGauge(t, 1, sink)
		testTimer(t, 1, sink)
	}
}

func TestSinkMap_Values(t *testing.T) {
	expCounters := make(map[string]uint64)
	expGauges := make(map[string]uint64)
	expTimers := make(map[string]float64)

	sink := mock.NewSink()
	for i := 0; i < 2; i++ {
		expCounters[fmt.Sprintf("counter-%d", i)] = uint64(i)
		expGauges[fmt.Sprintf("gauge-%d", i)] = uint64(i)
		expTimers[fmt.Sprintf("timer-%d", i)] = float64(i)

		sink.FlushCounter(fmt.Sprintf("counter-%d", i), uint64(i))
		sink.FlushGauge(fmt.Sprintf("gauge-%d", i), uint64(i))
		sink.FlushTimer(fmt.Sprintf("timer-%d", i), float64(i))
	}
	counters := sink.Counters()
	if !reflect.DeepEqual(expCounters, counters) {
		t.Errorf("Counters: want: %#v got: %#v", expCounters, counters)
	}

	gauges := sink.Gauges()
	if !reflect.DeepEqual(expGauges, gauges) {
		t.Errorf("Gauges: want: %#v got: %#v", expGauges, gauges)
	}

	timers := sink.Timers()
	if !reflect.DeepEqual(expTimers, timers) {
		t.Errorf("Timers: want: %#v got: %#v", expTimers, timers)
	}

	sink.Reset()
	if n := len(sink.Counters()); n != 0 {
		t.Errorf("Failed to reset Counters got: %d", n)
	}
	if n := len(sink.Gauges()); n != 0 {
		t.Errorf("Failed to reset Gauges got: %d", n)
	}
	if n := len(sink.Timers()); n != 0 {
		t.Errorf("Failed to reset Timers got: %d", n)
	}
}

// Test that the zero Sink is ready for use.
func TestSinkLazyInit(t *testing.T) {
	var s mock.Sink
	s.FlushCounter("counter", 1)
	s.AssertCounterEquals(t, "counter", 1)
}

func TestFlushTimer(t *testing.T) {
	sink := mock.NewSink()
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
	sink := mock.NewSink()
	var (
		counterCalls = new(int64)
		gaugeCalls   = new(int64)
		timerCalls   = new(int64)
		counterVal   = new(uint64)
	)
	funcs := [...]func(){
		func() {
			atomic.AddInt64(counterCalls, 1)
			atomic.AddUint64(counterVal, 1)
			sink.FlushCounter("name", 1)
		},
		func() {
			atomic.AddInt64(gaugeCalls, 1)
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
	sink.AssertGaugeEquals(t, "name", uint64(1))
}

func TestSink_ThreadSafe_Reset(t *testing.T) {
	const N = 2000
	sink := mock.NewSink()
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
	sink := mock.NewSink()
	sink.FlushCounter("name", 1)
	sink.AssertCounterEquals(mock.Fatal(t), "name", 1)
}

func setupBenchmark(prefix string) (*mock.Sink, [128]string) {
	var names [128]string
	if prefix == "" {
		prefix = "mock_sink"
	}
	for i := 0; i < len(names); i++ {
		names[i] = fmt.Sprintf("%s_%d", prefix, i)
	}
	sink := mock.NewSink()
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
