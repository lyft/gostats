package stats

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tagspkg "github.com/lyft/gostats/internal/tags"
	"github.com/lyft/gostats/mock"
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

func randomString(tb testing.TB, size int) string {
	b := make([]byte, hex.DecodedLen(size))
	if _, err := crand.Read(b); err != nil {
		tb.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func randomTagSet(t testing.TB, valPrefix string, size int) tagspkg.TagSet {
	s := make(tagspkg.TagSet, size)
	for i := 0; i < len(s); i++ {
		s[i] = tagspkg.NewTag(randomString(t, 32), fmt.Sprintf("%s%d", valPrefix, i))
	}
	s.Sort()
	return s
}

func TestNewSubScope(t *testing.T) {
	s := randomTagSet(t, "x_", 20)
	for i := range s {
		s[i].Value += "|" // add an invalid char
	}
	m := make(map[string]string)
	for _, p := range s {
		m[p.Key] = p.Value
	}
	scope := newSubScope(nil, "name", m)

	expected := make(tagspkg.TagSet, len(s))
	for i, p := range s {
		expected[i] = tagspkg.NewTag(p.Key, p.Value)
	}

	if !reflect.DeepEqual(scope.tags, expected) {
		t.Errorf("tags are not sorted by key: %+v", s)
	}
	for i, p := range expected {
		s := tagspkg.ReplaceChars(p.Value)
		if p.Value != s {
			t.Errorf("failed to replace invalid chars: %d: %+v", i, p)
		}
	}
	if scope.name != "name" {
		t.Errorf("wrong scope name: %s", scope.name)
	}
}

// Test that we never modify the tags map that is passed in
func TestTagMapNotModified(t *testing.T) {
	type TagMethod func(scope Scope, name string, tags map[string]string)

	copyTags := func(tags map[string]string) map[string]string {
		orig := make(map[string]string, len(tags))
		for k, v := range tags {
			orig[k] = v
		}
		return orig
	}

	scopeGenerators := map[string]func() Scope{
		"statStore": func() Scope { return &statStore{} },
		"subScope":  func() Scope { return newSubScope(&statStore{}, "name", nil) },
	}

	methodTestCases := map[string]TagMethod{
		"ScopeWithTags": func(scope Scope, name string, tags map[string]string) {
			scope.ScopeWithTags(name, tags)
		},
		"NewCounterWithTags": func(scope Scope, name string, tags map[string]string) {
			scope.NewCounterWithTags(name, tags)
		},
		"NewPerInstanceCounter": func(scope Scope, name string, tags map[string]string) {
			scope.NewPerInstanceCounter(name, tags)
		},
		"NewGaugeWithTags": func(scope Scope, name string, tags map[string]string) {
			scope.NewGaugeWithTags(name, tags)
		},
		"NewPerInstanceGauge": func(scope Scope, name string, tags map[string]string) {
			scope.NewPerInstanceGauge(name, tags)
		},
		"NewTimerWithTags": func(scope Scope, name string, tags map[string]string) {
			scope.NewTimerWithTags(name, tags)
		},
		"NewPerInstanceTimer": func(scope Scope, name string, tags map[string]string) {
			scope.NewPerInstanceTimer(name, tags)
		},
	}

	tagsTestCases := []map[string]string{
		{}, // empty
		{
			"": "invalid_key",
		},
		{
			"invalid_value": "",
		},
		{
			"":              "invalid_key",
			"invalid_value": "",
		},
		{
			"_f": "i",
		},
		{
			"":              "invalid_key",
			"invalid_value": "",
			"_f":            "i",
		},
		{
			"":              "invalid_key",
			"invalid_value": "",
			"_f":            "value",
			"1":             "1",
		},
		{
			"":              "invalid_key",
			"invalid_value": "",
			"1":             "1",
			"2":             "2",
			"3":             "3",
		},
	}

	for scopeName, newScope := range scopeGenerators {
		for methodName, method := range methodTestCases {
			t.Run(scopeName+"."+methodName, func(t *testing.T) {
				for _, orig := range tagsTestCases {
					tags := copyTags(orig)
					method(newScope(), "test", tags)
					if !reflect.DeepEqual(tags, orig) {
						t.Errorf("modified input map: %+v want: %+v", tags, orig)
					}
				}
			})
		}

	}
}

func TestPerInstanceStats(t *testing.T) {
	testCases := []struct {
		expected string
		tags     map[string]string
	}{
		{
			expected: "name.___f=i",
			tags:     map[string]string{}, // empty
		},
		{
			expected: "name.___f=i",
			tags: map[string]string{
				"": "invalid_key",
			},
		},
		{
			expected: "name.___f=i",
			tags: map[string]string{
				"invalid_value": "",
			},
		},
		{
			expected: "name.___f=i",
			tags: map[string]string{
				"_f": "i",
			},
		},
		{
			expected: "name.___f=xxx",
			tags: map[string]string{
				"_f": "xxx",
			},
		},
		{
			expected: "name.___f=xxx",
			tags: map[string]string{
				"":   "invalid_key",
				"_f": "xxx",
			},
		},
		{
			expected: "name.___f=xxx",
			tags: map[string]string{
				"invalid_value": "",
				"_f":            "xxx",
			},
		},
		{
			expected: "name.___f=xxx",
			tags: map[string]string{
				"invalid_value": "",
				"":              "invalid_key",
				"_f":            "xxx",
			},
		},
		{
			expected: "name.__1=1.___f=xxx",
			tags: map[string]string{
				"invalid_value": "",
				"":              "invalid_key",
				"_f":            "xxx",
				"1":             "1",
			},
		},
		{
			expected: "name.__1=1.___f=i",
			tags: map[string]string{
				"1": "1",
			},
		},
		{
			expected: "name.__1=1.__2=2.___f=i",
			tags: map[string]string{
				"1": "1",
				"2": "2",
			},
		},
	}


	testPerInstanceMethods := func(t *testing.T, setupScope func(Scope) Scope) {
		for _, x := range testCases {
			sink := mock.NewSink()
			scope := setupScope(&statStore{sink: sink})

			scope.NewPerInstanceCounter("name", x.tags).Inc()
			scope.NewPerInstanceGauge("name", x.tags).Inc()
			scope.NewPerInstanceTimer("name", x.tags).AddValue(1)
			scope.Store().Flush()

			for key := range sink.Counters() {
				if key != x.expected {
					t.Errorf("Counter (%+v): got: %q want: %q", x, key, x.expected)
				}
				break
			}

			for key := range sink.Gauges() {
				if key != x.expected {
					t.Errorf("Gauge (%+v): got: %q want: %q", x, key, x.expected)
				}
				break
			}

			for key := range sink.Timers() {
				if key != x.expected {
					t.Errorf("Timer (%+v): got: %q want: %q", x, key, x.expected)
				}
				break
			}
		}
	}

	t.Run("StatsStore", func(t *testing.T) {
		testPerInstanceMethods(t, func(scope Scope) Scope { return scope })
	})

	t.Run("SubScope", func(t *testing.T) {
		// Add sub-scope prefix to the name
		for i, x := range testCases {
			testCases[i].expected = "x." + x.expected
		}

		testPerInstanceMethods(t, func(scope Scope) Scope {
			return scope.Scope("x")
		})
	})
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

func BenchmarkParallelCounter(b *testing.B) {
	const N = 1000
	keys := make([]string, N)
	for i := 0; i < len(keys); i++ {
		keys[i] = randomString(b, 32)
	}

	s := NewStore(nullSink{}, false)
	t := time.NewTicker(time.Hour) // don't flush
	defer t.Stop()                 // never sends
	go s.Start(t)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		n := 0
		for pb.Next() {
			s.NewCounter(keys[n%N]).Inc()
		}
	})
}

func BenchmarkStoreNewPerInstanceCounter(b *testing.B) {
	b.Run("HasTag", func(b *testing.B) {
		var store statStore
		tags := map[string]string{
			"1":  "1",
			"2":  "2",
			"3":  "3",
			"_f": "xxx",
		}
		for i := 0; i < b.N; i++ {
			store.NewPerInstanceCounter("name", tags)
		}
	})

	b.Run("MissingTag", func(b *testing.B) {
		var store statStore
		tags := map[string]string{
			"1": "1",
			"2": "2",
			"3": "3",
			"4": "4",
		}
		for i := 0; i < b.N; i++ {
			store.NewPerInstanceCounter("name", tags)
		}
	})
}
