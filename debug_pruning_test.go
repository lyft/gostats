package stats

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

// TEMPORARY (OBSX-1114): tests for debug_pruning.go. Delete with it.

type debugLine struct {
	Msg  string            `json:"msg"`
	JSON map[string]string `json:"json"`
}

func readDebugLines(t *testing.T, buf *bytes.Buffer) []debugLine {
	t.Helper()
	var lines []debugLine
	dec := json.NewDecoder(buf)
	for dec.More() {
		var l debugLine
		if err := dec.Decode(&l); err != nil {
			t.Fatalf("decode log line: %v", err)
		}
		lines = append(lines, l)
	}
	return lines
}

func checkFields(t *testing.T, got debugLine, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if got.JSON[k] != v {
			t.Errorf("%s = %q, want %q (line: %+v)", k, got.JSON[k], v, got)
		}
	}
}

func newDebugStore(t *testing.T, pruneIdleSecs string, every time.Duration) (*statStore, *bytes.Buffer) {
	t.Helper()
	reset := testSetenv(t,
		"GOSTATS_PRUNE_IDLE_SECONDS", pruneIdleSecs,
		"GOSTATS_FLUSH_INTERVAL_SECONDS", "5",
	)
	t.Cleanup(reset)
	s := NewStore(nullSink{}, false).(*statStore)
	var buf bytes.Buffer
	s.debug.out = &buf
	s.debug.every = every
	return s, &buf
}

func TestPruningDebugLog(t *testing.T) {
	s, buf := newDebugStore(t, "10", 0) // 10s / 5s = prune after 2 idle flushes

	s.NewCounter("c").Inc()
	s.NewTimer("t").AddValue(1)
	s.NewGauge("g").Set(1)
	for i := 0; i < 3; i++ {
		s.Flush()
	}

	lines := readDebugLines(t, buf)
	if len(lines) != 3 {
		t.Fatalf("got %d log lines, want 3: %+v", len(lines), lines)
	}
	for _, l := range lines {
		if l.Msg != "gostats idle pruning" {
			t.Errorf("msg = %q, want %q", l.Msg, "gostats idle pruning")
		}
	}
	checkFields(t, lines[0], map[string]string{
		"prune_after_flushes": "2",
		"live_counters":       "1",
		"live_timers":         "1",
		"live_gauges":         "1",
		"pruned_counters":     "0",
		"pruned_timers":       "0",
	})
	checkFields(t, lines[2], map[string]string{
		"live_counters":   "0",
		"live_timers":     "0",
		"live_gauges":     "1",
		"pruned_counters": "1",
		"pruned_timers":   "1",
	})
}

// Between lines, pruned counts add up so none are lost to the interval.
func TestPruningDebugLogAccumulatesBetweenLines(t *testing.T) {
	s, buf := newDebugStore(t, "10", time.Hour)

	s.NewCounter("c1")
	s.NewCounter("c2")
	s.Flush() // logs: first line
	s.Flush() // prunes both, but inside the interval: no line
	s.debug.lastLog = time.Now().Add(-2 * time.Hour)
	s.Flush() // logs the prunes from the previous flush

	lines := readDebugLines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2: %+v", len(lines), lines)
	}
	checkFields(t, lines[1], map[string]string{
		"live_counters":   "0",
		"pruned_counters": "2",
	})
}

// With pruning off, log once so a missing env var shows up as a line
// rather than as silence.
func TestPruningDebugLogDisabled(t *testing.T) {
	s, buf := newDebugStore(t, "", 0)

	for i := 0; i < 3; i++ {
		s.Flush()
	}

	lines := readDebugLines(t, buf)
	if len(lines) != 1 || lines[0].Msg != "gostats idle pruning disabled" {
		t.Fatalf("got %+v, want exactly one %q line", lines, "gostats idle pruning disabled")
	}
}

// Stores built without NewStore (as most tests here do) have no writer
// and must not panic.
func TestPruningDebugLogOffForStructLiteral(_ *testing.T) {
	s := &statStore{sink: nullSink{}, pruneAfterFlushes: 2}
	s.NewCounter("c")
	for i := 0; i < 3; i++ {
		s.Flush()
	}
}
