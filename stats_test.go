package stats

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func testMergePairs(t *testing.T, base, sub map[string]string) {
	t.Helper()

	s := subScope{
		tags:  base,
		pairs: newTagSet(base),
	}

	var expected tagSet
	for k, v := range s.mergeTags(sub) {
		expected = append(expected, tagPair{
			dimension: k,
			value:     v,
		})
	}
	sort.Sort(expected)

	pairs := s.mergePairs(sub)
	if !reflect.DeepEqual(expected, pairs) {
		t.Logf("Base: %v\n", base)
		t.Logf("Sub: %v\n", sub)
		t.Errorf("Expected (%d): %v Got (%d): %v",
			len(expected), expected, len(pairs), pairs)
	}
}

func TestMergePairs(t *testing.T) {

	t.Run("Simple", func(t *testing.T) {
		base := map[string]string{
			"tag1": "val1",
			"tag2": "val2",
			"tag5": "val5",
		}
		sub := map[string]string{
			"tag1": "sub1",
			"tagX": "subX",
			"tag5": "sub5",
		}
		testMergePairs(t, base, sub)
	})

	t.Run("Equal_Pairs", func(t *testing.T) {
		base := map[string]string{
			"tag1": "val1",
			"tag2": "val2",
			"tag3": "val3",
		}
		sub := map[string]string{
			"tag1": "val1",
			"tag2": "val2",
			"tag3": "val3",
		}
		testMergePairs(t, base, sub)
	})

	t.Run("Small", func(t *testing.T) {
		base := map[string]string{
			"tag1": "val1",
		}
		sub := map[string]string{
			"tag1": "sub1",
		}
		testMergePairs(t, base, sub)
	})

	t.Run("None", func(t *testing.T) {
		base := map[string]string{}
		sub := map[string]string{}
		testMergePairs(t, base, sub)
	})

	makeTags := func(prefix string, n int) map[string]string {
		m := make(map[string]string, n)
		for i := 0; i < n; i++ {
			k := fmt.Sprintf("%s_key%d", prefix, i)
			v := fmt.Sprintf("%s_val%d", prefix, i)
			m[k] = v
		}
		return m
	}

	rr := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("Random_%d", i), func(t *testing.T) {
			base := makeTags("base", rr.Intn(50))
			sub := makeTags("sub", rr.Intn(50))

			// interleave the maps so that sub has some identical
			// keys and some keys with different values
			var (
				equalKV = 5 // same pair
				sameK   = 5 // same key - different value
			)
			for k, v := range base {
				switch {
				case equalKV > 0:
					sub[k] = v
					equalKV--
				case sameK > 0:
					sub[k] = v + "_X"
					sameK--
				}
			}
			testMergePairs(t, base, sub)
		})
	}
}

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

func initBenchScope() (scope *subScope, childTags map[string]string) {
	s := NewStore(nullSink{}, false)

	t := time.NewTicker(time.Hour) // don't flush
	t.Stop()                       // never sends
	go s.Start(t)

	scopeTags := make(map[string]string, 5)
	childTags = make(map[string]string, 5)

	for i := 0; i < 5; i++ {
		tag := fmt.Sprintf("%dtag", i)
		val := fmt.Sprintf("%dval", i)
		scopeTags[tag] = val
		childTags["c"+tag] = "c" + val
	}

	scope = s.ScopeWithTags("scope", scopeTags).(*subScope)
	return
}

func BenchmarkStore_ScopeWithTags(b *testing.B) {
	// scope, childTags := initBenchScope()
	scope, _ := initBenchScope()
	b.ResetTimer()
	n := 0
	x := strconv.Itoa(n)
	for i := 0; i < b.N; i++ {
		// scope.NewCounterWithTags("counter_name", childTags).Add(1)
		scope.NewCounterWithTags("counter_name", map[string]string{
			"tag1": "sub1",
			"tagX": "subX",
			"tag5": "sub5",
			"X":    x,
		}).Add(1)
		if i%256 == 0 {
			n++
			x = strconv.Itoa(n)
		}
	}
}

func BenchmarkStore_ScopeWithTags_Pairs(b *testing.B) {
	// scope, childTags := initBenchScope()
	scope, _ := initBenchScope()
	scope.pairs = newTagSet(scope.tags)
	b.ResetTimer()
	n := 0
	x := strconv.Itoa(n)
	for i := 0; i < b.N; i++ {
		// scope.NewCounterWithTags("counter_name", childTags).Add(1)
		scope.NewCounterWithTags_X("counter_name", map[string]string{
			"tag1": "sub1",
			"tagX": "subX",
			"tag5": "sub5",
			"X":    x,
		}).Add(1)
		if i%256 == 0 {
			n++
			x = strconv.Itoa(n)
		}
	}
}

func BenchmarkStore_ScopeNoTags(b *testing.B) {
	scope, _ := initBenchScope()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scope.NewCounterWithTags("counter_name", nil)
	}
}

func randomKey(tb testing.TB) string {
	b := make([]byte, 64)
	if _, err := io.ReadFull(crand.Reader, b); err != nil {
		tb.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func BenchmarkParallelCounter(b *testing.B) {
	const N = 1000
	keys := make([]string, N)
	for i := 0; i < len(keys); i++ {
		keys[i] = randomKey(b)
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

func BenchmarkMergeTags(b *testing.B) {
	// base := map[string]string{
	// 	"tag1": "val1",
	// 	"tag2": "val2",
	// 	"tag3": "val3",
	// 	"tag4": "val4",
	// 	"tag5": "val5",
	// }
	base := make(map[string]string)
	for i := 0; i < 6; i++ {
		k := fmt.Sprintf("%dtag", i)
		v := fmt.Sprintf("%dval", i)
		base[k] = v
	}
	s := subScope{
		tags:  base,
		pairs: newTagSet(base),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.mergeTags(map[string]string{
			"tag1": "sub1",
			"tagX": "subX",
			"tag5": "sub5",
		})
	}
}

func BenchmarkMergePairs(b *testing.B) {
	// base := map[string]string{
	// 	"tag1": "val1",
	// 	"tag2": "val2",
	// 	"tag3": "val3",
	// 	"tag4": "val4",
	// 	"tag5": "val5",
	// }
	base := make(map[string]string)
	for i := 0; i < 6; i++ {
		k := fmt.Sprintf("%dtag", i)
		v := fmt.Sprintf("%dval", i)
		base[k] = v
	}
	s := subScope{
		tags:  base,
		pairs: newTagSet(base),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.mergePairs(map[string]string{
			"tag1": "sub1",
			"tagX": "subX",
			"tag5": "sub5",
		})
	}
}
