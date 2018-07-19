package stats

import (
	"fmt"
	"github.com/golang/mock/gomock"
	"strings"
	"sync"
	"testing"
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

func NewTestObjects(t *testing.T) (*gomock.Controller, *Mockdialer, *Mockconn, *tcpStatsdSink) {
	ctrl := gomock.NewController(t)
	d := NewMockdialer(ctrl)
	c := NewMockconn(ctrl)
	c.EXPECT().Write(gomock.Any()).Times(10).DoAndReturn(func(buf []byte) (int, error) {
		return len(buf), nil
	})
	d.EXPECT().Dial("tcp", gomock.Any()).Times(1).Return(c, nil)
	s := newTCPStatsdSink(d, make(chan struct{}))
	return ctrl, d, c, s
}

func TestFuckMe(t *testing.T) {
	_, _, _, s := NewTestObjects(t)
	for i := 0; i < 10; i++ {
		s.FlushGauge("sup", 42)
		<-s.writtenChan
	}
}
