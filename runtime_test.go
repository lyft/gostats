package stats

import (
	"regexp"
	"testing"
)

func TestRuntime(t *testing.T) {
	sink := &testStatSink{}
	store := NewStore(sink, true)

	scope := store.Scope("runtime")
	g := NewRuntimeStats(scope)
	g.GenerateStats()
	store.Flush()

	pattern :=
		`runtime.lastGC:\d+|g\n` +
			`runtime.pauseTotalNs:\d+|g\n` +
			`runtime.numGC:\d+|g\n` +
			`runtime.alloc:\d+|g\n` +
			`runtime.totalAlloc:\d+|g\n` +
			`runtime.frees:\d+|g\n` +
			`runtime.nextGC:\d+|g\n`

	ok, err := regexp.Match(pattern, []byte(sink.record))
	if err != nil {
		t.Fatal("Error: ", err)
	}
	if !ok {
		t.Errorf("Expected: '%s' Got: '%s'", pattern, sink.record)
	}
}
