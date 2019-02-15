package stats

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"strconv"
	"testing"
)

func TestFlushableSink_Flush(t *testing.T) {
	var buf bytes.Buffer
	s := flushableSink{
		w:   &buf,
		buf: bufio.NewWriter(&buf),
	}
	t.Run("Counter", func(t *testing.T) {
		defer buf.Reset()
		s.FlushCounter("counter", 1234)
		s.Flush()
		const exp = "counter:1234|c\n"
		if s := buf.String(); s != exp {
			t.Errorf("Counter: want: %q got: %q", exp, s)
		}
	})
	t.Run("Gauge", func(t *testing.T) {
		defer buf.Reset()
		s.FlushGauge("gauge", 1234)
		s.Flush()
		const exp = "gauge:1234|g\n"
		if s := buf.String(); s != exp {
			t.Errorf("Gauge: want: %q got: %q", exp, s)
		}
	})
	t.Run("Timer", func(t *testing.T) {
		defer buf.Reset()
		s.FlushTimer("timer_int", 1234)
		s.Flush()
		const exp = "timer_int:1234|ms\n"
		if s := buf.String(); s != exp {
			t.Errorf("Timer: want: %q got: %q", exp, s)
		}
	})
	t.Run("Timer_Fraction", func(t *testing.T) {
		defer buf.Reset()
		s.FlushTimer("timer_frac", 1234.12)
		s.Flush()
		const exp = "timer_frac:1234.12|ms\n"
		if s := buf.String(); s != exp {
			t.Errorf("Timer: want: %q got: %q", exp, s)
		}
	})
	t.Run("Timer_Fmt", func(t *testing.T) {
		defer buf.Reset()
		s.FlushTimer("timer_frac", 1234.12)
		s.Flush()
		exp := fmt.Sprintf("%s:%f|ms\n", "timer_frac", 1234.12)
		if s := buf.String(); s == exp {
			t.Errorf("Timer: we should not be using fmt's default float formatting: %q", s)
		}
	})
}

func TestFlushableSink_FlushFloatLimits(t *testing.T) {
	const MaxInt = 1 << 53
	var buf bytes.Buffer
	s := flushableSink{
		w:   &buf,
		buf: bufio.NewWriter(&buf),
	}
	// make sure we use floating point when the value cannot be
	// represented as a uint64
	ff := float64(math.MaxUint64 - 1000)
	for i := 0; i < 10000; i++ {
		exp := fmt.Sprintf("timer:%s|ms\n", strconv.FormatFloat(ff, 'f', -1, 64))
		s.FlushTimer("timer", ff)
		s.Flush()
		if s := buf.String(); s != exp {
			t.Fatalf("FlushFloat (%d - %f): want: %q got: %q", i, ff, exp, s)
		}
		buf.Reset()
		ff++
	}
}

type nopWriter struct{}

func (nopWriter) Write(b []byte) (int, error) { return len(b), nil }

func BenchmarkFlushableSink_FlushUint_Baseline(b *testing.B) {
	const name = "fairly_descriptive_stat_name.child.__tag1=value1"
	var w nopWriter
	for i := 0; i < b.N; i++ {
		fmt.Fprintf(w, "%s:%d|c\n", name, uint64(i))
	}
}

func BenchmarkFlushableSink_FlushUint(b *testing.B) {
	const name = "fairly_descriptive_stat_name.child.__tag1=value1"
	s := flushableSink{
		w:   nopWriter{},
		buf: bufio.NewWriter(nopWriter{}),
	}
	for i := 0; i < b.N; i++ {
		s.FlushCounter(name, uint64(i))
	}
}

func BenchmarkFlushableSink_FlushFloat_Baseline_Frac(b *testing.B) {
	const name = "fairly_descriptive_stat_name.child.__tag1=value1"
	var w nopWriter
	for i := 0; i < b.N; i++ {
		fmt.Fprintf(w, "%s:%f|ms\n", name, float64(i)/7)
	}
}

func BenchmarkFlushableSink_FlushFloat_Frac(b *testing.B) {
	const name = "fairly_descriptive_stat_name.child.__tag1=value1"
	s := flushableSink{
		w:   nopWriter{},
		buf: bufio.NewWriter(nopWriter{}),
	}
	for i := 0; i < b.N; i++ {
		s.FlushTimer(name, float64(i)/7)
	}
}

func BenchmarkFlushableSink_FlushFloat_Baseline_Int(b *testing.B) {
	const name = "fairly_descriptive_stat_name.child.__tag1=value1"
	var w nopWriter
	for i := 0; i < b.N; i++ {
		fmt.Fprintf(w, "%s:%f|ms\n", name, float64(i))
	}
}

func BenchmarkFlushableSink_FlushFloat_Int(b *testing.B) {
	const name = "fairly_descriptive_stat_name.child.__tag1=value1"
	s := flushableSink{
		w:   nopWriter{},
		buf: bufio.NewWriter(nopWriter{}),
	}
	for i := 0; i < b.N; i++ {
		s.FlushTimer(name, float64(i))
	}
}
