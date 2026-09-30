package stats

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

// TEMPORARY (OBSX-1114): logs idle-pruning activity so a service trying
// lyft/gostats#189 can see whether pruning runs and how many entries the
// store holds. Delete this file, debug_pruning_test.go, and the calls to
// it in stats.go before #189 merges.

// pruningDebug is only touched from Flush, which flushMu serializes.
type pruningDebug struct {
	out   io.Writer // nil turns logging off
	every time.Duration

	lastLog        time.Time
	loggedDisabled bool
	prunedCounters uint64 // since the last line
	prunedTimers   uint64
}

func newPruningDebug() pruningDebug {
	return pruningDebug{out: os.Stderr, every: time.Minute}
}

func (s *statStore) logPruning(prunedCounters, prunedTimers uint64) {
	d := &s.debug
	if d.out == nil {
		return
	}
	if s.pruneAfterFlushes == 0 {
		if !d.loggedDisabled {
			d.loggedDisabled = true
			s.writeDebug("gostats idle pruning disabled", map[string]string{})
		}
		return
	}
	d.prunedCounters += prunedCounters
	d.prunedTimers += prunedTimers
	now := time.Now()
	if !d.lastLog.IsZero() && now.Sub(d.lastLog) < d.every {
		return
	}
	d.lastLog = now
	s.writeDebug("gostats idle pruning", map[string]string{
		"prune_after_flushes": strconv.FormatUint(uint64(s.pruneAfterFlushes), 10),
		"live_counters":       strconv.Itoa(syncMapLen(&s.counters)),
		"live_timers":         strconv.Itoa(syncMapLen(&s.timers)),
		"live_gauges":         strconv.Itoa(syncMapLen(&s.gauges)),
		"pruned_counters":     strconv.FormatUint(d.prunedCounters, 10),
		"pruned_timers":       strconv.FormatUint(d.prunedTimers, 10),
	})
	d.prunedCounters, d.prunedTimers = 0, 0
}

// writeDebug uses the same JSON shape as loggingSink. "store" tells apart
// the lines from services that build more than one store per process.
func (s *statStore) writeDebug(msg string, fields map[string]string) {
	fields["store"] = fmt.Sprintf("%p", s)
	nanos := time.Now().UnixNano()
	json.NewEncoder(s.debug.out).Encode(logLine{
		Level:     "info",
		Timestamp: sixDecimalPlacesFloat(float64(nanos) / float64(time.Second)),
		Logger:    "gostats.pruning",
		Message:   msg,
		JSON:      fields,
	})
}

func syncMapLen(m *sync.Map) int {
	n := 0
	m.Range(func(_, _ interface{}) bool {
		n++
		return true
	})
	return n
}
