package stats

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type testStatSink struct {
	sync.Mutex
	record string
}

func (s *testStatSink) FlushCounter(name string, value uint64) {
	s.Lock()
	s.record += fmt.Sprintf("%s:%d|c\n", name, value)
	s.Unlock()
}

func (s *testStatSink) FlushGauge(name string, value uint64) {
	s.Lock()
	s.record += fmt.Sprintf("%s:%d|g\n", name, value)
	s.Unlock()
}

func (s *testStatSink) FlushTimer(name string, value float64) {
	s.Lock()
	s.record += fmt.Sprintf("%s:%f|ms\n", name, value)
	s.Unlock()
}

func TestCreateTimer(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	t1 := store.NewTimer("hello")
	if t1 == nil {
		t.Fatal("No timer returned")
	}

	t2 := store.NewTimer("hello")
	if t1 != t2 {
		t.Error("A new timer with the same name was returned")
	}
}

func TestCreateTaggedTimer(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	t1 := store.NewTimerWithTags("hello", map[string]string{"t1": "v1"})
	if t1 == nil {
		t.Fatal("No timer returned")
	}

	t2 := store.NewTimerWithTags("hello", map[string]string{"t1": "v1"})
	if t1 != t2 {
		t.Error("A new timer with the same name was returned")
	}
}

func TestCreateInstanceTimer(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	t1 := store.NewPerInstanceTimer("hello", map[string]string{"t1": "v1"})
	if t1 == nil {
		t.Fatal("No timer returned")
	}

	t2 := store.NewPerInstanceTimer("hello", map[string]string{"t1": "v1"})
	if t1 != t2 {
		t.Error("A new timer with the same name was returned")
	}

	span := t2.AllocateSpan()
	span.Complete()

	expected := "hello.___f=i.__t1=v1"
	if !strings.Contains(sink.record, expected) {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestCreateInstanceTimerNilMap(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	t1 := store.NewPerInstanceTimer("hello", nil)
	if t1 == nil {
		t.Fatal("No timer returned")
	}

	t2 := store.NewPerInstanceTimer("hello", map[string]string{"_f": "i"})
	if t1 != t2 {
		t.Error("A new timer with the same name was returned")
	}

	span := t2.AllocateSpan()
	span.Complete()

	expected := "hello.___f=i"
	if !strings.Contains(sink.record, expected) {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestCreateDuplicateCounter(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	t1 := store.NewCounter("TestCreateCounter")
	if t1 == nil {
		t.Fatal("No counter returned")
	}

	t2 := store.NewCounter("TestCreateCounter")
	if t1 != t2 {
		t.Error("A new counter with the same name was returned")
	}
}

func TestCounter(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	counter := store.NewCounter("c")
	counter.Inc()
	counter.Add(10)
	counter.Inc()
	store.Flush()

	expected := "c:12|c\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestTaggedPerInstanceCounter(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)
	scope := store.Scope("prefix")

	counter := scope.NewPerInstanceCounter("c", map[string]string{"tag1": "value1"})
	counter.Inc()
	counter.Add(10)
	counter.Inc()
	store.Flush()

	expected := "prefix.c.___f=i.__tag1=value1:12|c\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestTaggedPerInstanceCounterWithNilTags(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)
	scope := store.Scope("prefix")

	counter := scope.NewPerInstanceCounter("c", nil)
	counter.Inc()
	counter.Add(10)
	counter.Inc()
	store.Flush()

	expected := "prefix.c.___f=i:12|c\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestTaggedCounter(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)
	scope := store.Scope("prefix")

	counter := scope.NewCounterWithTags("c", map[string]string{"tag1": "value1"})
	counter.Inc()
	counter.Add(10)
	counter.Inc()
	store.Flush()

	expected := "prefix.c.__tag1=value1:12|c\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestCreateDuplicateGauge(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	t1 := store.NewGauge("TestCreateGauge")
	if t1 == nil {
		t.Fatal("No gauge returned")
	}

	t2 := store.NewGauge("TestCreateGauge")
	if t1 != t2 {
		t.Error("A new counter with the same name was returned")
	}
}

func TestGauge(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	gauge := store.NewGauge("TestGauge")
	gauge.Inc()
	gauge.Add(10)
	gauge.Dec()
	gauge.Sub(5)
	store.Flush()

	expected := "TestGauge:5|g\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestTaggedGauge(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	gauge := store.NewGaugeWithTags("TestGauge", map[string]string{"tag1": "v1"})
	gauge.Inc()
	gauge.Add(10)
	gauge.Dec()
	gauge.Sub(5)
	store.Flush()

	expected := "TestGauge.__tag1=v1:5|g\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestPerInstanceGauge(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	gauge := store.NewPerInstanceGauge("TestGauge", map[string]string{"tag1": "v1"})
	gauge.Inc()
	gauge.Add(10)
	gauge.Dec()
	gauge.Sub(5)
	store.Flush()

	expected := "TestGauge.___f=i.__tag1=v1:5|g\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestPerInstanceGaugeWithNilTags(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	gauge := store.NewPerInstanceGauge("TestGauge", nil)
	gauge.Inc()
	gauge.Add(10)
	gauge.Dec()
	gauge.Sub(5)
	store.Flush()

	expected := "TestGauge.___f=i:5|g\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestScopes(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	ascope := store.Scope("a")
	bscope := ascope.Scope("b")
	counter := bscope.NewCounter("c")
	counter.Inc()
	store.Flush()

	expected := "a.b.c:1|c\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestScopesWithTags(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	ascope := store.ScopeWithTags("a", map[string]string{"x": "a", "y": "a"})
	bscope := ascope.ScopeWithTags("b", map[string]string{"x": "b", "z": "b"})
	dscope := bscope.Scope("d")
	counter := dscope.NewCounter("c")
	counter.Inc()
	timer := dscope.NewTimer("t")
	timer.AddValue(1)
	gauge := dscope.NewGauge("g")
	gauge.Set(1)
	store.Flush()

	expected := "a.b.d.t.__x=b.__y=a.__z=b:1.000000|ms\na.b.d.c.__x=b.__y=a.__z=b:1|c\na.b.d.g.__x=b.__y=a.__z=b:1|g\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestScopesAndMetricsWithTags(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	ascope := store.ScopeWithTags("a", map[string]string{"x": "a", "y": "a"})
	bscope := ascope.Scope("b")
	counter := bscope.NewCounterWithTags("c", map[string]string{"x": "m", "z": "m"})
	counter.Inc()
	timer := bscope.NewTimerWithTags("t", map[string]string{"x": "m", "z": "m"})
	timer.AddValue(1)
	gauge := bscope.NewGaugeWithTags("g", map[string]string{"x": "m", "z": "m"})
	gauge.Set(1)
	store.Flush()

	expected := "a.b.t.__x=m.__y=a.__z=m:1.000000|ms\na.b.c.__x=m.__y=a.__z=m:1|c\na.b.g.__x=m.__y=a.__z=m:1|g\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

type testStatGenerator struct {
	counter Counter
	gauge   Gauge
}

func (s *testStatGenerator) GenerateStats() {
	s.counter.Add(123)
	s.gauge.Set(456)
}

func TestStatGenerator(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)
	scope := store.Scope("TestRuntime")

	g := testStatGenerator{counter: scope.NewCounter("counter"), gauge: scope.NewGauge("gauge")}

	store.AddStatGenerator(&g)
	store.Flush()

	expected := "TestRuntime.counter:123|c\nTestRuntime.gauge:456|g\n"
	if expected != sink.record {
		t.Errorf("Expected: '%s' Got: '%s'", expected, sink.record)
	}
}

func TestTCPStatsdSink_Flush(t *testing.T) {
	lc, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lc.Close()

	_, port, err := net.SplitHostPort(lc.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	oldPort, exists := os.LookupEnv("STATSD_PORT")
	if exists {
		defer os.Setenv("STATSD_PORT", oldPort)
	} else {
		defer os.Unsetenv("STATSD_PORT")
	}
	os.Setenv("STATSD_PORT", port)

	go func() {
		for {
			conn, err := lc.Accept()
			if conn != nil {
				_, _ = io.Copy(ioutil.Discard, conn)
			}
			if err != nil {
				return
			}
		}
	}()

	sink := NewTCPStatsdSink()

	t.Run("One", func(t *testing.T) {
		flushed := make(chan struct{})
		go func() {
			defer close(flushed)
			sink.Flush()
		}()
		select {
		case <-flushed:
			// ok
		case <-time.After(time.Second):
			t.Fatal("Flush blocked")
		}
	})

	t.Run("Ten", func(t *testing.T) {
		flushed := make(chan struct{})
		go func() {
			defer close(flushed)
			for i := 0; i < 10; i++ {
				sink.Flush()
			}
		}()
		select {
		case <-flushed:
			// ok
		case <-time.After(time.Second):
			t.Fatal("Flush blocked")
		}
	})

	t.Run("Parallel", func(t *testing.T) {
		start := make(chan struct{})
		wg := new(sync.WaitGroup)
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				<-start
				defer wg.Done()
				for i := 0; i < 10; i++ {
					sink.Flush()
				}
			}()
		}
		flushed := make(chan struct{})
		go func() {
			close(start)
			wg.Wait()
			close(flushed)
		}()
		select {
		case <-flushed:
			// ok
		case <-time.After(time.Second):
			t.Fatal("Flush blocked")
		}
	})
}

type nopWriter struct{}

func (nopWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func BenchmarkFlushCounter(b *testing.B) {
	sink := tcpStatsdSink{
		bufWriter: bufio.NewWriter(nopWriter{}),
	}
	for i := 0; i < b.N; i++ {
		sink.FlushCounter("TestCounter.___f=i.__tag1=v1", uint64(i))
	}
}

func BenchmarkFlushTimer(b *testing.B) {
	sink := tcpStatsdSink{
		bufWriter: bufio.NewWriter(nopWriter{}),
	}
	for i := 0; i < b.N; i++ {
		sink.FlushTimer("TestTImer.___f=i.__tag1=v1", float64(i)/3)
	}
}
