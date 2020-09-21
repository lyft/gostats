package stats

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	logger "github.com/sirupsen/logrus"
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
		t.Errorf("\n# Expected:\n%s\n# Got:\n%s\n", expected, sink.record)
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
		t.Errorf("\n# Expected:\n%s\n# Got:\n%s\n", expected, sink.record)
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

func TestNetSink_Flush(t *testing.T) {
	lc, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lc.Close()

	_, port, err := net.SplitHostPort(lc.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, err := lc.Accept()
			if err == nil {
				_, err = io.Copy(ioutil.Discard, conn)
			}
			if err != nil {
				return
			}
		}
	}()

	nport, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	sink := NewNetSink(
		WithLogger(discardLogger()),
		WithStatsdPort(nport),
	)

	// Spin up a goroutine to flood the sink with large Counter stats.
	// The goal here is to keep the buffer channel full.
	ready := make(chan struct{}, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		name := "test." + strings.Repeat("a", 1024) + ".counter"
		for {
			select {
			case ready <- struct{}{}:
				// signal that we've started
			case <-done:
				return
			default:
				for i := 0; i < 1000; i++ {
					sink.FlushCounter(name, 1)
				}
			}
		}
	}()
	<-ready // wait for goroutine to start

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

// Test that drainFlushQueue() does not hang when there are continuous
// flush requests.
func TestNetSink_DrainFlushQueue(t *testing.T) {
	s := &netSink{
		doFlush: make(chan chan struct{}, 8),
	}

	sent := new(int64)

	// Saturate the flush channel

	done := make(chan struct{})
	defer close(done)

	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			for {
				select {
				case s.doFlush <- make(chan struct{}):
					atomic.AddInt64(sent, 1)
				case <-done:
					return
				}
			}
		}()
	}

	// Wait for the flush channel to fill
	for len(s.doFlush) < cap(s.doFlush) {
		runtime.Gosched()
	}

	flushed := make(chan struct{})
	go func() {
		s.drainFlushQueue()
		close(flushed)
	}()

	// We will flush up to cap(s.doFlush) * 8 items, so the max number
	// of sends will be that plus the capacity of the buffer.
	maxSends := cap(s.doFlush)*8 + cap(s.doFlush)

	select {
	case <-flushed:
		n := int(atomic.LoadInt64(sent))
		switch {
		case n < cap(s.doFlush):
			// This should be impossible since we fill the channel
			// before calling drainFlushQueue().
			t.Errorf("Sent less than %d items: %d", cap(s.doFlush), n)
		case n > maxSends:
			// This should be nearly impossible to get without inserting
			// runtime.Gosched() into the flush/drain loop.
			t.Errorf("Sent more than %d items: %d", maxSends, n)
		}
	case <-time.After(time.Second / 2):
		// 500ms is really generous, it should return almost immediately.
		t.Error("drainFlushQueue did not return in time")
	}
}

func discardLogger() *logger.Logger {
	log := logger.New()
	log.Out = ioutil.Discard
	return log
}

func setupTestNetSink(t *testing.T, protocol string, stop bool) (*netTestSink, *netSink) {
	ts := newNetTestSink(t, protocol)

	if stop {
		if err := ts.Close(); err != nil {
			t.Fatal(err)
		}
	}

	sink := NewTCPStatsdSink(
		WithLogger(discardLogger()),
		WithStatsdHost(ts.Host(t)),
		WithStatsdPort(ts.Port(t)),
		WithStatsdProtocol(protocol),
	).(*netSink)

	return ts, sink
}

func testNetSinkBufferSize(t *testing.T, protocol string) {
	var size int
	switch protocol {
	case "udp":
		size = defaultBufferSizeUDP
	case "tcp":
		size = defaultBufferSizeTCP
	}
	ts, sink := setupTestNetSink(t, protocol, false)
	defer ts.Close()
	if sink.bufWriter.Size() != size {
		t.Errorf("Buffer Size: got: %d want: %d", sink.bufWriter.Size(), size)
	}
}

func testNetSinkStatTypes(t *testing.T, protocol string) {
	var expected = [...]string{
		"counter:1|c\n",
		"gauge:1|g\n",
		"timer_int:1|ms\n",
		"timer_float:1.230000|ms\n",
	}

	ts, sink := setupTestNetSink(t, protocol, false)
	defer ts.Close()

	sink.FlushCounter("counter", 1)
	sink.FlushGauge("gauge", 1)
	sink.FlushTimer("timer_int", 1)
	sink.FlushTimer("timer_float", 1.23)
	sink.Flush()

	for _, exp := range expected {
		stat := ts.WaitForStat(t, time.Millisecond*50)
		if stat != exp {
			t.Errorf("stats got: %q want: %q", stat, exp)
		}
	}

	// make sure there aren't any extra stats we're missing
	exp := strings.Join(expected[:], "")
	buf := ts.String()
	if buf != exp {
		t.Errorf("stats buffer\ngot:\n%q\nwant:\n%q\n", buf, exp)
	}
}

func testNetSinkImmediateFlush(t *testing.T, protocol string) {
	const expected = "counter:1|c\n"

	ts, sink := setupTestNetSink(t, protocol, false)
	defer ts.Close()

	sink.FlushCounter("counter", 1)
	sink.Flush()

	stat := ts.WaitForStat(t, time.Millisecond*50)
	if stat != expected {
		t.Errorf("stats got: %q want: %q", stat, expected)
	}
}

// replaceFatalWithLog replaces calls to t.Fatalf() with t.Logf().
type replaceFatalWithLog struct {
	*testing.T
}

func (t replaceFatalWithLog) Fatalf(format string, args ...interface{}) {
	t.Logf(format, args...)
}

func testNetSinkReconnect(t *testing.T, protocol string) {
	if testing.Short() {
		t.Skip("Skipping: short test")
	}
	t.Parallel()

	const expected = "counter:1|c\n"

	ts, sink := setupTestNetSink(t, protocol, true)
	defer ts.Close()

	sink.FlushCounter("counter", 1)

	flushed := make(chan struct{})
	go func() {
		flushed <- struct{}{}
		sink.Flush()
		close(flushed)
	}()

	<-flushed // wait till we're ready
	ts.Restart(t, true)

	sink.FlushCounter("counter", 1)

	// This test is flaky with UDP and the race detector, but good
	// to have so we log instead of fail the test.
	if protocol == "udp" {
		stat := ts.WaitForStat(replaceFatalWithLog{t}, defaultRetryInterval*3)
		if stat != "" && stat != expected {
			t.Fatalf("stats got: %q want: %q", stat, expected)
		}
	} else {
		stat := ts.WaitForStat(t, defaultRetryInterval*3)
		if stat != expected {
			t.Fatalf("stats got: %q want: %q", stat, expected)
		}
	}

	// Make sure our flush call returned
	select {
	case <-flushed:
	case <-time.After(time.Millisecond * 100):
		// The flushed channel should be closed by this point,
		// but this was failing in CI on go1.12 due to timing
		// issues so we relax the constraint and give it 100ms.
		t.Error("Flush() did not return")
	}
}

func testNetSinkReconnectFailure(t *testing.T, protocol string) {
	if testing.Short() {
		t.Skip("Skipping: short test")
	}
	t.Parallel()

	ts, sink := setupTestNetSink(t, protocol, true)
	defer ts.Close()

	sink.FlushCounter("counter", 1)

	const N = 16
	flushCount := new(int64)
	flushed := make(chan struct{})
	go func() {
		wg := new(sync.WaitGroup)
		wg.Add(N)
		for i := 0; i < N; i++ {
			go func() {
				sink.Flush()
				atomic.AddInt64(flushCount, 1)
				wg.Done()
			}()
		}
		wg.Wait()
		close(flushed)
	}()

	// Make sure our flush call returned
	select {
	case <-flushed:
		// Ok
	case <-time.After(defaultRetryInterval * 2):
		t.Fatalf("Only %d of %d Flush() calls succeeded",
			atomic.LoadInt64(flushCount), N)
	}
}

func TestNetSink_BufferSize_TCP(t *testing.T) {
	testNetSinkBufferSize(t, "tcp")
}

func TestNetSink_BufferSize_UDP(t *testing.T) {
	testNetSinkBufferSize(t, "udp")
}

func TestNetSink_StatTypes_TCP(t *testing.T) {
	testNetSinkStatTypes(t, "tcp")
}

func TestNetSink_StatTypes_UDP(t *testing.T) {
	testNetSinkStatTypes(t, "udp")
}

func TestNetSink_ImmediateFlush_TCP(t *testing.T) {
	testNetSinkImmediateFlush(t, "tcp")
}

func TestNetSink_ImmediateFlush_UDP(t *testing.T) {
	testNetSinkImmediateFlush(t, "udp")
}

func TestNetSink_Reconnect_TCP(t *testing.T) {
	testNetSinkReconnect(t, "tcp")
}

func TestNetSink_Reconnect_UDP(t *testing.T) {
	testNetSinkReconnect(t, "udp")
}

func TestNetSink_ReconnectFailure_TCP(t *testing.T) {
	testNetSinkReconnectFailure(t, "tcp")
}

func TestNetSink_ReconnectFailure_UDP(t *testing.T) {
	testNetSinkReconnectFailure(t, "udp")
}

func buildBinary(t testing.TB, path string) (string, func()) {
	var binaryName string
	if strings.HasSuffix(path, ".go") {
		// foo/bar/main.go => bar
		binaryName = filepath.Base(filepath.Dir(path))
	} else {
		filepath.Base(path)
	}

	tmpdir, err := ioutil.TempDir("", "gostats-")
	if err != nil {
		t.Fatalf("creating tempdir: %v", err)
	}
	output := filepath.Join(tmpdir, binaryName)

	out, err := exec.Command("go", "build", "-o", output, path).CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build %s: %s\n### output:\n%s\n###\n",
			path, err, strings.TrimSpace(string(out)))
	}

	cleanup := func() {
		os.RemoveAll(tmpdir)
	}
	return output, cleanup
}

func testNetSinkIntegration(t *testing.T, protocol string) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fastExitExe, deleteBinary := buildBinary(t, "testdata/fast_exit/fast_exit.go")
	defer deleteBinary()

	// Test the stats of a fast exiting program are captured.
	t.Run("FastExit", func(t *testing.T) {
		ts := newNetTestSink(t, protocol)
		defer ts.Close()

		cmd := exec.CommandContext(ctx, fastExitExe)
		cmd.Env = ts.CommandEnv(t)

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Running command: %s\n### output:\n%s\n###\n",
				fastExitExe, strings.TrimSpace(string(out)))
		}

		stats := ts.String()
		const expected = "test.fast.exit.counter:1|c\n"
		if stats != expected {
			t.Errorf("stats: got: %q want: %q", stats, expected)
		}
	})

	// Test that Flush() does not hang if the TCP sink is in a reconnect loop
	t.Run("Reconnect", func(t *testing.T) {
		ts := newNetTestSink(t, protocol)
		defer ts.Close()

		cmd := exec.CommandContext(ctx, fastExitExe)
		cmd.Env = ts.CommandEnv(t)

		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		errCh := make(chan error, 1)
		go func() { errCh <- cmd.Wait() }()

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(defaultRetryInterval * 2):
			t.Fatal("Timed out waiting for command to exit")
		}
	})
}

func TestNetSink_Integration_TCP(t *testing.T) {
	testNetSinkIntegration(t, "tcp")
}

func TestNetSink_Integration_UDP(t *testing.T) {
	testNetSinkIntegration(t, "udp")
}

type nopWriter struct{}

func (nopWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func BenchmarkFlushCounter(b *testing.B) {
	sink := netSink{
		bufWriter: bufio.NewWriter(nopWriter{}),
	}
	for i := 0; i < b.N; i++ {
		sink.FlushCounter("TestCounter.___f=i.__tag1=v1", uint64(i))
	}
}

func BenchmarkFlushTimer(b *testing.B) {
	sink := netSink{
		bufWriter: bufio.NewWriter(nopWriter{}),
	}
	for i := 0; i < b.N; i++ {
		sink.FlushTimer("TestTImer.___f=i.__tag1=v1", float64(i)/3)
	}
}
