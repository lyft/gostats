package stats

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// This test will generate a lot of "statsd channel full" errors
	// so silence the logger.  This also means that this test cannot
	// be ran in parallel.
	logger.SetOutput(ioutil.Discard)
	defer logger.SetOutput(os.Stderr)

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
func TestTCPStatsdSink_DrainFlushQueue(t *testing.T) {
	s := &tcpStatsdSink{
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

type tcpTestSink struct {
	ll    *net.TCPListener
	addr  *net.TCPAddr
	mu    sync.Mutex // buf lock
	buf   bytes.Buffer
	stats chan string
	done  chan struct{} // closed when read loop exits
}

func newTCPTestSink(t testing.TB) *tcpTestSink {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	})
	if err != nil {
		t.Fatal("ListenTCP:", err)
	}
	s := &tcpTestSink{
		ll:    l,
		addr:  l.Addr().(*net.TCPAddr),
		stats: make(chan string, 64),
		done:  make(chan struct{}),
	}
	go s.run(t)
	return s
}

func (s *tcpTestSink) writeStat(line []byte) {
	select {
	case s.stats <- string(line):
	default:
	}
	s.mu.Lock()
	s.buf.Write(line)
	s.mu.Unlock()
}

func (s *tcpTestSink) run(t testing.TB) {
	defer close(s.done)
	buf := bufio.NewReader(nil)
	for {
		conn, err := s.ll.AcceptTCP()
		if err != nil {
			// Log errors other than poll.ErrNetClosing, which is an
			// internal error so we have to match against it's string.
			if !strings.Contains(err.Error(), "use of closed network connection") {
				t.Logf("Error: accept: %v", err)
			}
			return
		}
		// read stats line by line
		buf.Reset(conn)
		for {
			b, e := buf.ReadBytes('\n')
			if len(b) > 0 {
				s.writeStat(b)
			}
			if e != nil {
				if e != io.EOF {
					err = e
				}
				break
			}
		}
		if buf.Buffered() != 0 {
			buf.WriteTo(&s.buf)
		}
		if err != nil {
			t.Errorf("Error: reading stats: %v", err)
		}
	}
}

func (s *tcpTestSink) Restart(t testing.TB, resetBuffer bool) {
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	l, err := net.ListenTCP(s.addr.Network(), s.addr)
	if err != nil {
		t.Fatalf("restarting connection: %v", err)
	}
	if resetBuffer {
		s.buf.Reset()
	}
	*s = tcpTestSink{
		ll:    l,
		addr:  s.addr,
		buf:   s.buf,
		stats: make(chan string, 64),
		done:  make(chan struct{}),
	}
	go s.run(t)
}

func (s *tcpTestSink) Close() error {
	select {
	case <-s.done:
		return nil // closed
	default:
		return s.ll.Close()
	}
}

func (s *tcpTestSink) WaitForStat(t testing.TB, timeout time.Duration) string {
	t.Helper()
	if timeout <= 0 {
		timeout = defaultRetryInterval * 2
	}
	to := time.NewTimer(timeout)
	defer to.Stop()
	select {
	case s := <-s.stats:
		return s
	case <-to.C:
		t.Fatalf("timeout waiting to receive stat: %s", timeout)
	}
	return ""
}

func (s *tcpTestSink) Stats() <-chan string {
	return s.stats
}

func (s *tcpTestSink) Bytes() []byte {
	s.mu.Lock()
	b := append([]byte(nil), s.buf.Bytes()...)
	s.mu.Unlock()
	return b
}

func (s *tcpTestSink) String() string {
	s.mu.Lock()
	str := s.buf.String()
	s.mu.Unlock()
	return str
}

func (s *tcpTestSink) Address() *net.TCPAddr {
	return s.addr
}

func mergeEnv(extra ...string) []string {
	var prefixes []string
	for _, s := range extra {
		n := strings.IndexByte(s, '=')
		prefixes = append(prefixes, s[:n+1])
	}
	ignore := func(s string) bool {
		for _, pfx := range prefixes {
			if strings.HasPrefix(s, pfx) {
				return true
			}
		}
		return false
	}

	env := os.Environ()
	a := env[:0]
	for _, s := range env {
		if !ignore(s) {
			a = append(a, s)
		}
	}
	return append(a, extra...)
}

// CommandEnv returns the environment variables for an *exec.Cmd to use
// with this test sink.
func (s *tcpTestSink) CommandEnv() []string {
	return mergeEnv(
		fmt.Sprintf("STATSD_PORT=%d", s.Address().Port),
		fmt.Sprintf("STATSD_HOST=%s", s.Address().IP.String()),
		"GOSTATS_FLUSH_INTERVAL_SECONDS=1",
	)
}

func discardLogger() *logger.Logger {
	log := logger.New()
	log.Out = ioutil.Discard
	return log
}

func TestTCPStatsdSink(t *testing.T) {
	setup := func(t *testing.T, stop bool) (*tcpTestSink, *tcpStatsdSink) {
		ts := newTCPTestSink(t)

		if stop {
			if err := ts.Close(); err != nil {
				t.Fatal(err)
			}
		}

		sink := NewTCPStatsdSink(
			WithLogger(discardLogger()),
			WithStatsdHost(ts.Address().IP.String()),
			WithStatsdPort(ts.Address().Port),
		).(*tcpStatsdSink)

		return ts, sink
	}

	t.Run("StatTypes", func(t *testing.T) {
		var expected = [...]string{
			"counter:1|c\n",
			"gauge:1|g\n",
			"timer_int:1|ms\n",
			"timer_float:1.230000|ms\n",
		}

		ts, sink := setup(t, false)
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
	})

	// Make sure that stats are immediately flushed so that stats from fast
	// exiting programs are not lost.
	t.Run("ImmediateFlush", func(t *testing.T) {
		const expected = "counter:1|c\n"

		ts, sink := setup(t, false)
		defer ts.Close()

		sink.FlushCounter("counter", 1)
		sink.Flush()

		stat := ts.WaitForStat(t, time.Millisecond*50)
		if stat != expected {
			t.Errorf("stats got: %q want: %q", stat, expected)
		}
	})

	// Test that we can successfully reconnect and flush a stat.
	t.Run("Reconnect", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping: short test")
		}
		t.Parallel()

		const expected = "counter:1|c\n"

		ts, sink := setup(t, true)
		defer ts.Close()

		sink.FlushCounter("counter", 1)

		flushed := make(chan struct{})
		go func() {
			sink.Flush()
			close(flushed)
		}()

		time.Sleep(time.Millisecond * 100)
		ts.Restart(t, true)

		stat := ts.WaitForStat(t, defaultRetryInterval*2)
		if stat != expected {
			t.Fatalf("stats got: %q want: %q", stat, expected)
		}

		// Make sure our flush call returned
		select {
		case <-flushed:
		default:
			t.Error("Flush() did not return")
		}
	})

	// Test that when reconnecting fails, calls the Flush() do not block
	// indefinitely.
	t.Run("ReconnectFailure", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping: short test")
		}
		t.Parallel()

		ts, sink := setup(t, true)
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
	})
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

func TestTCPStatsdSink_Integration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fastExitExe, deleteBinary := buildBinary(t, "testdata/fast_exit/fast_exit.go")
	defer deleteBinary()

	// Test the stats of a fast exiting program are captured.
	t.Run("FastExit", func(t *testing.T) {
		ts := newTCPTestSink(t)
		defer ts.Close()

		cmd := exec.CommandContext(ctx, fastExitExe)
		cmd.Env = ts.CommandEnv()

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
		ts := newTCPTestSink(t)
		defer ts.Close()

		cmd := exec.CommandContext(ctx, fastExitExe)
		cmd.Env = ts.CommandEnv()

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
