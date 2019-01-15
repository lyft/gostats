package stats

import (
	"fmt"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Ensure flushing and adding generators does not race
func TestStats(t *testing.T) {
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

func testNewGaugeWithTags_ExpvarPublish(t *testing.T, N int) {
	tags := map[string]string{
		"tag1": "var1",
		"tag2": "var2",
		"tag3": "var3",
		"tag4": "var4",
	}

	store := NewStore(nullSink{}, true)
	wg := new(sync.WaitGroup)
	start := new(sync.WaitGroup)
	start.Add(1)

	numCPU := runtime.NumCPU() * 4 // increase lock contention
	for i := 0; i < numCPU; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start.Wait()
			for i := 0; i < N; i++ {
				store.NewGaugeWithTags("gauge.foo.bar.baz."+strconv.Itoa(i), tags)
			}
		}()
	}
	start.Done()
	wg.Wait()
}

// Test that concurrently creating new gauges does not call expvar.Publish()
// with the same name twice.
func TestNewGaugeWithTags_ExpvarPublish(t *testing.T) {
	const N = 200
	if testing.Short() {
		t.Log("skipping full test in short mode")
		testNewGaugeWithTags_ExpvarPublish(t, N)
		return
	}
	// this test typically fails early so run multiple times
	// instead of running once with a larger value for N
	for i := 0; i < 10; i++ {
		testNewGaugeWithTags_ExpvarPublish(t, N)
	}
}

func testNewCounterWithTags_ExpvarPublish(t *testing.T, N int) {
	tags := map[string]string{
		"tag1": "var1",
		"tag2": "var2",
		"tag3": "var3",
		"tag4": "var4",
	}

	store := NewStore(nullSink{}, true)
	wg := new(sync.WaitGroup)
	start := new(sync.WaitGroup)
	start.Add(1)

	numCPU := runtime.NumCPU() * 4 // increase lock contention
	for i := 0; i < numCPU; i++ {
		wg.Add(1)
		go func() {
			defer func() {
				if e := recover(); e != nil {
					t.Fatalf("Duplicate calls to expvar.Publish: %v", e)
				}
			}()
			defer wg.Done()
			start.Wait()
			for i := 0; i < N; i++ {
				store.NewCounterWithTags("counter.foo.bar.baz."+strconv.Itoa(i), tags)
			}
		}()
	}
	start.Done()
	wg.Wait()
}

// Test that concurrently creating new gauges does not call expvar.Publish()
// with the same name twice.
func TestNewCounterWithTags_ExpvarPublish(t *testing.T) {
	const N = 200
	if testing.Short() {
		t.Log("skipping full test in short mode")
		testNewCounterWithTags_ExpvarPublish(t, N)
		return
	}
	// this test typically fails early so run multiple times
	// instead of running once with a larger value for N
	for i := 0; i < 10; i++ {
		testNewCounterWithTags_ExpvarPublish(t, N)
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

	t := time.NewTicker(time.Hour) // don't flush
	t.Stop()                       // never sends
	go s.Start(t)

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
