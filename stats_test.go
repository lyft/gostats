package stats

import (
	"bufio"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
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

	expected := make(tagSet, len(s))
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

func mergeTagsReference(s *subScope, tags map[string]string) tagSet {
	a := make(tagSet, 0, len(tags))
	for k, v := range tags {
		if k != "" && v != "" {
			a = append(a, tagPair{key: k, value: replaceChars(v)})
		}
	}
	return mergeTagSetsReference(s.tags, a)
}

func tagMapsEqual(m1, m2 map[string]string) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k, v := range m1 {
		if vv, ok := m2[k]; !ok || vv != v {
			return false
		}
	}
	return true
}

func testMergeTags(t *testing.T, s1, s2 tagSet, perInstanceTag bool) {
	tags := make(map[string]string, len(s2))
	origTags := make(map[string]string, len(s2))
	for _, p := range s2 {
		tags[p.key] = p.value
		origTags[p.key] = p.value
	}

	scope := subScope{tags: s1}
	origPairs := append(tagSet(nil), scope.tags...)
	expected := mergeTagsReference(&scope, tags)

	var got tagSet
	if perInstanceTag {
		if !expected.Contains("_f") {
			expected = expected.Insert(tagPair{key: "_f", value: "i"})
		}
		got = scope.mergePerInstanceTags(tags)
	} else {
		got = scope.mergeTags(tags)
	}
	if !tagSetEqual(got, expected) {
		t.Errorf("{s1=%d, s2=%d}: bad merge:\n# Got:\n%+v\n\n# Want:\n%+v\n",
			len(s1), len(s2), got, expected)
	}
	if !tagSetEqual(scope.tags, origPairs) {
		t.Fatalf("scope tags modified:\n# Got:\n%+v\n\n# Want:\n%+v\n",
			scope.tags, origPairs)
	}
	if !tagMapsEqual(tags, origTags) {
		t.Fatalf("tag map modified:\n# Got:\n%v\n\n# Want:\n%v\n",
			scope.tags, origPairs)
	}
	if perInstanceTag {
		if !got.Contains("_f") {
			t.Fatal("missing per-instance tag")
		}
		exp := "i" // default
		for _, p := range expected {
			if p.key == "_f" {
				exp = replaceChars(p.value)
				break
			}
		}
		tag := got[got.Search("_f")].value
		if tag != exp {
			t.Fatalf("per-instance tag want: %q got: %q: %+v", exp, tag, got)
		}
	}
}

func TestMergePerInstanceTags(t *testing.T) {
	t.Parallel()

	rr := rand.New(rand.NewSource(time.Now().UnixNano()))

	type testCase struct {
		n1, n2            int
		hasPerInstanceTag bool
	}
	tests := make([]testCase, 0, 2100)

	// make sure we cover all 0..2 test cases
	for i := 0; i <= 2; i++ {
		for j := 0; j <= 2; j++ {
			for k := 0; k < 2; k++ {
				tests = append(tests, testCase{i, j, k == 1})
			}
		}
	}
	// add a whole bunch of random cases
	for i := 0; i < 2000; i++ {
		tests = append(tests, testCase{rr.Intn(8), rr.Intn(8), false})
	}

	for _, x := range tests {
		s1 := randomTagSet(t, "v", x.n1)
		s2 := randomTagSet(t, "v", x.n2)
		if x.hasPerInstanceTag {
			s1 = s1.Insert(tagPair{key: "_f", value: "foo"})
		}
		if rr.Float64() < 0.1 {
			s1 = s1.Insert(tagPair{key: "_f", value: "foo"})
		}
		if rr.Float64() < 0.1 {
			s2 = s2.Insert(tagPair{key: "_f", value: "bar"})
		}

		// Add some invalid chars to s2
		for i := range s2 {
			if rr.Float64() < 0.2 {
				s2[i].value += "|"
			}
			if rr.Float64() < 0.1 {
				s2[i].value = ""
			}
			if rr.Float64() < 0.1 {
				s2[i].key = ""
			}
		}
		testMergeTags(t, s1, s2, true)
	}
}

func TestMergeTags(t *testing.T) {
	t.Parallel()

	rr := rand.New(rand.NewSource(time.Now().UnixNano()))

	type testCase struct {
		n1, n2 int
	}
	tests := make([]testCase, 0, 2100)

	// make sure we cover all 0..2 test cases
	for i := 0; i <= 2; i++ {
		for j := 0; j <= 2; j++ {
			tests = append(tests, testCase{i, j})
		}
	}
	// add a whole bunch of random cases
	for i := 0; i < 2000; i++ {
		tests = append(tests, testCase{rr.Intn(64), rr.Intn(64)})
	}

	for _, x := range tests {
		s1 := randomTagSet(t, "v", x.n1)
		s2 := randomTagSet(t, "v", x.n2)

		// Add some invalid chars to s2
		for i := range s2 {
			if rr.Float64() < 0.2 {
				s2[i].value += "|"
			}
			if rr.Float64() < 0.1 {
				s2[i].value = ""
			}
			if rr.Float64() < 0.1 {
				s2[i].key = ""
			}
		}
		testMergeTags(t, s1, s2, false)
	}
}

func TestMergeOneTagPanic(t *testing.T) {
	tags := map[string]string{
		"k1": "v1",
		"k2": "v2",
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic got none")
		}
	}()
	var scope subScope
	scope.mergeOneTag(tags)
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

var randReader = struct {
	*bufio.Reader
	*sync.Mutex
}{
	Reader: bufio.NewReaderSize(crand.Reader, 1024*64),
	Mutex:  new(sync.Mutex),
}

func RandomString(tb testing.TB, size int) string {
	b := make([]byte, hex.DecodedLen(size))
	randReader.Lock()
	defer randReader.Unlock()
	if _, err := io.ReadFull(randReader, b); err != nil {
		tb.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func BenchmarkParallelCounter(b *testing.B) {
	const N = 1000
	keys := make([]string, N)
	for i := 0; i < len(keys); i++ {
		keys[i] = RandomString(b, 32)
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

// TODO: rename this once we rename the mergePairs method
func benchScopeMergeTags(b *testing.B, baseSize, tagsSize int) {
	s := subScope{
		tags: make(tagSet, baseSize),
	}
	for i := range s.tags {
		s.tags[i] = tagPair{
			key:   fmt.Sprintf("key1_%d", i),
			value: fmt.Sprintf("val1_%d", i),
		}
	}
	tags := make(map[string]string, tagsSize)
	for i := 0; i < tagsSize; i++ {
		key := fmt.Sprintf("key2_%d", i)
		val := fmt.Sprintf("val2_%d", i)
		tags[key] = val
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		s.mergeTags(tags)
	}
}

// TODO: rename this once we rename the mergePairs method
func BenchmarkScopeMergeTags(b *testing.B) {
	if testing.Short() {
		b.Skip("short test")
	}
	for baseSize := 1; baseSize <= 8; baseSize++ {
		for tagSize := 1; tagSize <= 8; tagSize++ {
			b.Run(fmt.Sprintf("%d_%d", baseSize, tagSize), func(b *testing.B) {
				benchScopeMergeTags(b, baseSize, tagSize)
			})
		}
	}
}
