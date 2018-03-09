package stats

import (
	"math/rand"
	"strconv"
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
	sink := NewMockSink()
	store := NewStore(sink, true)
	store.NewTimer("test").AllocateSpan().CompleteWithDuration(testDuration)

	expected := uint64(98)
	timer, ok := sink.Timers["test"]
	if !ok {
		t.Errorf("wanted a %q timer, none found", "test")
	} else if timer != expected {
		t.Error("wanted 1000, got", timer)
	}
}

var bmID = ""
var bmVal = uint64(0)

func BenchmarkStore_MutexContention(b *testing.B) {
	s := NewStore(&nullSink{}, false)
	t := time.NewTicker(500 * time.Microsecond) // we want flush to contend with accessing metrics
	defer t.Stop()
	go s.Start(t)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bmID = strconv.Itoa(rand.Intn(1000))
		c := s.NewCounter(bmID)
		c.Inc()
		bmVal = c.Value()
	}
}
