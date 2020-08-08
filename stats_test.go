package stats

import (
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestNewSubScope(t *testing.T) {
	s := randomTagSet(t, "x_", 20)
	for i := range s {
		s[i].value += "|" // add an invalid char
	}
	m := make(map[string]string)
	for _, p := range s {
		m[p.key] = p.value
	}
	scope := newSubScope(nil, "name", m)

	expected := make(TagSet, len(s))
	copy(expected, s)
	for i, p := range expected {
		expected[i].value = replaceChars(p.value)
	}

	if !reflect.DeepEqual(scope.tags, expected) {
		t.Errorf("tags are not sorted by key: %+v", s)
	}
	for i, p := range expected {
		s := replaceChars(p.value)
		if p.value != s {
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

	sink := mock.NewSink()

	testPerInstanceMethods := func(t *testing.T, scope Scope) {
		for _, x := range testCases {
			sink.Reset()

			scope.NewPerInstanceCounter("name", x.tags).Inc()
			scope.Store().Flush()
			for key := range sink.Counters() {
				if key != x.expected {
					t.Errorf("Counter (%+v): got: %q want: %q", x, key, x.expected)
				}
				break
			}

			scope.NewPerInstanceGauge("name", x.tags).Inc()
			scope.Store().Flush()
			for key := range sink.Counters() {
				if key != x.expected {
					t.Errorf("Gauge (%+v): got: %q want: %q", x, key, x.expected)
				}
				break
			}

			scope.NewPerInstanceTimer("name", x.tags).AddValue(1)
			scope.Store().Flush()
			for key := range sink.Counters() {
				if key != x.expected {
					t.Errorf("Timer (%+v): got: %q want: %q", x, key, x.expected)
				}
				break
			}
		}
	}

	t.Run("StatsStore", func(t *testing.T) {
		store := &statStore{sink: sink}

		testPerInstanceMethods(t, store)
	})

	t.Run("SubScope", func(t *testing.T) {
		store := &subScope{registry: &statStore{sink: sink}, name: "x"}

		// Add sub-scope prefix to the name
		for i, x := range testCases {
			testCases[i].expected = "x." + x.expected
		}

		testPerInstanceMethods(t, store)
	})
}
