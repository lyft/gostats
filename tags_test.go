package stats

import (
	"bufio"
	"bytes"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Reference serializeTags implementation
func serializeTagsReference(name string, tags map[string]string) string {
	const prefix = ".__"
	const sep = "="
	if len(tags) == 0 {
		return name
	}
	tagPairs := make([]Tag, 0, len(tags))
	for tagKey, tagValue := range tags {
		tagValue = replaceChars(tagValue)
		tagPairs = append(tagPairs, Tag{tagKey, tagValue})
	}
	sort.Sort(TagSet(tagPairs))

	buf := new(bytes.Buffer)
	for _, tag := range tagPairs {
		if tag.key != "" && tag.value != "" {
			fmt.Fprint(buf, prefix, tag.key, sep, tag.value)
		}
	}
	return name + buf.String()
}

func TestSerializeTags(t *testing.T) {
	const name = "prefix"
	const expected = name + ".__q=r.__zzz=hello"
	tags := map[string]string{"zzz": "hello", "q": "r"}
	serialized := serializeTags(name, tags)
	if serialized != expected {
		t.Errorf("Serialized output (%s) didn't match expected output: %s",
			serialized, expected)
	}
}

// Test that the optimized serializeTags() function matches the reference
// implementation.
func TestSerializeTagsReference(t *testing.T) {
	const name = "prefix"
	makeTags := func(n int) map[string]string {
		m := make(map[string]string, n)
		for i := 0; i < n; i++ {
			k := fmt.Sprintf("key%d", i)
			v := fmt.Sprintf("val%d", i)
			m[k] = v
		}
		return m
	}
	for i := 0; i < 100; i++ {
		tags := makeTags(i)
		expected := serializeTagsReference(name, tags)
		serialized := serializeTags(name, tags)
		if serialized != expected {
			t.Errorf("%d Serialized output (%s) didn't match expected output: %s",
				i, serialized, expected)
		}
	}
}

// Test the network sort used when we have 4 or less tags.  Since the iteration
// order of maps is random we use random keys in an attempt to get 100% test
// coverage.
func TestSerializeTagsNetworkSort(t *testing.T) {
	const name = "prefix"

	rand.Seed(time.Now().UnixNano())
	buf := bufio.NewReader(crand.Reader)
	seen := make(map[string]bool)

	randomString := func() string {
		for i := 0; i < 100; i++ {
			b := make([]byte, rand.Intn(30)+1)
			if _, err := buf.Read(b); err != nil {
				t.Fatal(err)
			}
			s := base64.StdEncoding.EncodeToString(b)
			if !seen[s] {
				seen[s] = true
				return s
			}
		}
		t.Fatal("Failed to generate a random string")
		return ""
	}

	makeTags := func(n int) map[string]string {
		m := make(map[string]string, n)
		for i := 0; i < n; i++ {
			k := randomString()
			v := randomString()
			m[k] = v
		}
		return m
	}

	// we use a network sort when tag length is 4 or less, but test up to 8
	// here in case that value is ever increased.
	for i := 1; i <= 4; i++ {
		// loop to increase the odds of 100% test coverage
		for i := 0; i < 10; i++ {
			tags := makeTags(i)
			expected := serializeTagsReference(name, tags)
			serialized := serializeTags(name, tags)
			if serialized != expected {
				t.Errorf("%d Serialized output (%s) didn't match expected output: %s",
					i, serialized, expected)
			}
		}
	}
}

func TestSerializeTagsInvalidKeyValue(t *testing.T) {

	// Baseline tests against a hardcoded expected value
	t.Run("Baseline", func(t *testing.T) {
		const expected = "name.__1=1"
		tags := map[string]string{
			"":              "invalid_key",
			"invalid_value": "",
			"1":             "1",
		}
		orig := make(map[string]string)
		for k, v := range tags {
			orig[k] = v
		}

		s := serializeTags("name", tags)
		if s != expected {
			t.Errorf("Serialized output (%s) didn't match expected output: %s",
				s, expected)
		}

		if !reflect.DeepEqual(tags, orig) {
			t.Errorf("serializeTags modified the input map: %+v want: %+v", tags, orig)
		}
	})

	createTags := func(n int) map[string]string {
		tags := make(map[string]string)
		for i := 0; i < n; i++ {
			key := fmt.Sprintf("key_%d", i)
			val := fmt.Sprintf("val_%d", i)
			tags[key] = val
		}
		return tags
	}

	test := func(t *testing.T, tags map[string]string) {
		orig := make(map[string]string)
		for k, v := range tags {
			orig[k] = v
		}

		got := serializeTags("name", tags)
		exp := serializeTagsReference("name", tags)
		if got != exp {
			t.Errorf("Tags (%d) got: %q want: %q", len(tags), got, exp)
		}

		if !reflect.DeepEqual(tags, orig) {
			t.Errorf("serializeTags modified the input map: %+v want: %+v", tags, orig)
		}
	}

	t.Run("EmptyValue", func(t *testing.T) {
		for n := 0; n <= 10; n++ {
			tags := createTags(n)
			tags["invalid"] = ""
			test(t, tags)
		}
	})

	t.Run("EmptyKey", func(t *testing.T) {
		for n := 0; n <= 10; n++ {
			tags := createTags(n)
			tags[""] = "invalid"
			test(t, tags)
		}
	})

	t.Run("EmptyKeyValue", func(t *testing.T) {
		for n := 0; n <= 10; n++ {
			tags := createTags(n)
			tags[""] = "invalid"
			tags["invalid"] = ""
			test(t, tags)
		}
	})
}

func TestSerializeTagsInvalidKeyValue_ThreadSafe(t *testing.T) {
	tags := map[string]string{
		"":  "invalid_key",
		"1": "1",
	}
	// Add some more keys to slow this down
	for i := 0; i < 256; i++ {
		v := "val_" + strconv.Itoa(i)
		k := "key_" + v
		tags[k] = v
	}

	// Make a copy
	orig := make(map[string]string, len(tags))
	for k, v := range tags {
		orig[k] = v
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU()*2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			serializeTags("name", tags)
		}()
	}
	close(start)
	wg.Wait()

	if !reflect.DeepEqual(tags, orig) {
		t.Error("serializeTags modified the input map")
	}
}

func TestSerializeWithPerInstanceFlag(t *testing.T) {
	const name = "prefix"
	const expected = name + ".___f=i.__foo=bar"
	tags := map[string]string{"foo": "bar", "_f": "i"}
	serialized := serializeTags(name, tags)
	if serialized != expected {
		t.Errorf("Serialized output (%s) didn't match expected output: %s",
			serialized, expected)
	}
}

func TestSerializeIllegalTags(t *testing.T) {
	const name = "prefix"
	const expected = name + ".__foo=b_a_r.__q=p"
	tags := map[string]string{"foo": "b|a:r", "q": "p"}
	serialized := serializeTags(name, tags)
	if serialized != expected {
		t.Errorf("Serialized output (%s) didn't match expected output: %s",
			serialized, expected)
	}
}

func TestSerializeTagValuePeriod(t *testing.T) {
	const name = "prefix"
	const expected = name + ".__foo=blah_blah.__q=p"
	tags := map[string]string{"foo": "blah.blah", "q": "p"}
	serialized := serializeTags(name, tags)
	if serialized != expected {
		t.Errorf("Serialized output (%s) didn't match expected output: %s",
			serialized, expected)
	}
}

func TestSerializeTagDiscardEmptyTagKeyValue(t *testing.T) {
	const name = "prefix"
	const expected = name + ".__key1=value1.__key3=value3"
	tags := map[string]string{"key1": "value1", "key2": "", "key3": "value3", "": "value4"}
	serialized := serializeTags(name, tags)
	if serialized != expected {
		t.Errorf("Serialized output (%s) didn't match expected output: %s",
			serialized, expected)
	}
}

func TestTagSort(t *testing.T) {
	contains := func(key string, tags TagSet) bool {
		for _, t := range tags {
			if t.key == key {
				return true
			}
		}
		return false
	}

	for n := 0; n < 20; n++ {
		tags := randomTagSet(t, "v", n)
		keys := make([]string, 0, len(tags)+5)
		for _, t := range tags {
			keys = append(keys, t.key)
		}
		for i := 0; i < 5; i++ {
			for {
				s := RandomString(t, 10)
				if !contains(s, tags) {
					keys = append(keys, RandomString(t, 10))
					break
				}
			}
		}
		for _, key := range keys {
			i := tags.Search(key)
			j := sort.Search(len(tags), func(i int) bool {
				return tags[i].key >= key
			})
			if i != j {
				t.Errorf("%d: Search got: %d want: %d", n, i, j)
			}
		}

		for _, key := range keys {
			exp := contains(key, tags)
			got := tags.Contains(key)
			if exp != got {
				t.Errorf("%d: tags contains (%q) want: %t got: %t", n, key, exp, got)
			}
		}

		for i := range tags {
			j := tags.Search(tags[i].key)
			if j != i {
				t.Errorf("%d: search did not find %q-%d: %d", n, tags[i].key, i, j)
			}
		}
	}
}

func randomTagSet(t testing.TB, valPrefix string, size int) TagSet {
	s := make(TagSet, size)
	for i := 0; i < len(s); i++ {
		s[i] = Tag{
			key:   RandomString(t, 32),
			value: fmt.Sprintf("%s%d", valPrefix, i),
		}
	}
	s.Sort()
	return s
}

func TestTagInsert(t *testing.T) {
	t1 := randomTagSet(t, "t1_", 1000)
	t2 := randomTagSet(t, "t2_", 1000)
	if !sort.IsSorted(t1) {
		t.Fatal("tags being inserted into must be sorted!")
	}
	for i := range t2 {
		t1 = t1.Insert(t2[i])
		if !sort.IsSorted(t1) {
			t.Fatalf("%d: inserting tag failed: %+v", i, t2[i])
		}
	}
}

func mergeTagSetsReference(s1, s2 TagSet) TagSet {
	seen := make(map[string]bool)
	var a TagSet
	for _, t := range s2 {
		a = append(a, t)
		seen[t.key] = true
	}
	for _, t := range s1 {
		if !seen[t.key] {
			a = append(a, t)
		}
	}
	a.Sort()
	return a
}

func makeScratch(s1, s2 TagSet) (TagSet, TagSet) {
	a := make(TagSet, len(s1)+len(s2))
	copy(a[len(s1):], s2)
	return a, a[len(s1):]
}

func tagSetEqual(s1, s2 TagSet) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

func TestMergeTagSets(t *testing.T) {
	for i := 0; i < 100; i++ {
		s1 := randomTagSet(t, "s1_", i)
		s2 := randomTagSet(t, "s2_", i)
		for i := 0; i < len(s2); i++ {
			if i&1 == 0 {
				s2[i] = s1[i]
			}
		}
		s1.Sort()
		s2.Sort()
		if !sort.IsSorted(s1) {
			t.Fatal("s1 not sorted")
		}
		if !sort.IsSorted(s2) {
			t.Fatal("s2 not sorted")
		}

		expected := mergeTagSetsReference(s1, s2)
		a, to := makeScratch(s1, s2)
		got := mergeTagSets(s1, to, a)
		if !sort.IsSorted(got) {
			t.Errorf("merging %d tagSets failed: not sorted", i)
		}
		if !tagSetEqual(got, expected) {
			// t.Errorf("merging %d tagSets", i)
			t.Errorf("merging %d tagSets failed\n# Got:\n%+v\n# Want:\n%+v\n# S1:\n%+v\n# S2:\n%+v\n",
				i, got, expected, s1, s2)
		}
	}
}

func benchmarkSerializeTags(b *testing.B, n int) {
	const name = "prefix"
	tags := make(map[string]string, n)
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("key%d", i)
		v := fmt.Sprintf("val%d", i)
		tags[k] = v
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		serializeTags(name, tags)
	}
}

func BenchmarkSerializeTags(b *testing.B) {
	for i := 1; i <= 10; i++ {
		b.Run(fmt.Sprintf("%d", i), func(b *testing.B) {
			benchmarkSerializeTags(b, i)
		})
	}
}

func benchmarkSerializeTagSet(b *testing.B, n int) {
	const name = "prefix"
	tags := make(TagSet, 0, n)
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("key%d", i)
		v := fmt.Sprintf("val%d", i)
		tags = append(tags, Tag{
			key:   k,
			value: v,
		})
	}
	tags.Sort()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		serializeTagSet(name, tags)
	}
}

func BenchmarkSerializeTagSet(b *testing.B) {
	for i := 1; i <= 10; i++ {
		b.Run(fmt.Sprintf("%d", i), func(b *testing.B) {
			benchmarkSerializeTagSet(b, i)
		})
	}
}

// TODO (CEV): consider removing this
func BenchmarkTagSearch(b *testing.B) {
	rr := rand.New(rand.NewSource(12345))
	tags := make(TagSet, 5)
	for i := range tags {
		tags[i].key = strconv.FormatInt(rr.Int63(), 10)
		tags[i].value = strconv.FormatInt(rr.Int63(), 10)
	}
	tags.Sort()

	for i := 0; i < b.N; i++ {
		for _, tag := range tags {
			_ = tags.Search(tag.key)
		}
	}
}

// TODO (CEV): consider removing this
func BenchmarkTagSearch_Reference(b *testing.B) {
	rr := rand.New(rand.NewSource(12345))
	tags := make(TagSet, 5)
	for i := range tags {
		tags[i].key = strconv.FormatInt(rr.Int63(), 10)
		tags[i].value = strconv.FormatInt(rr.Int63(), 10)
	}
	tags.Sort()

	for i := 0; i < b.N; i++ {
		for _, tag := range tags {
			_ = sort.Search(len(tags), func(i int) bool {
				return tags[i].key >= tag.key
			})
		}
	}
}

func benchTagSort(b *testing.B, size int) {
	// CEV: this isn't super accurate since we also time
	// the copying the orig slice into the test slice,
	// but its still useful.

	// use a fixed source so that results are comparable
	rr := rand.New(rand.NewSource(12345))
	orig := make(TagSet, size)
	for i := range orig {
		orig[i].key = strconv.FormatInt(rr.Int63(), 10)
		orig[i].value = strconv.FormatInt(rr.Int63(), 10)
	}
	rr.Shuffle(len(orig), func(i, j int) {
		orig[i], orig[j] = orig[j], orig[i]
	})
	tags := make(TagSet, len(orig))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(tags, orig)
		tags.Sort()
	}
}

func BenchmarkTagSort(b *testing.B) {
	if testing.Short() {
		b.Skip("short test")
	}
	for i := 2; i <= 10; i++ {
		b.Run(fmt.Sprint(i), func(b *testing.B) {
			benchTagSort(b, i)
		})
	}
}

// TODO: consider making this benchmark smaller
func BenchmarkMergeTagSets(b *testing.B) {
	if testing.Short() {
		b.Skip("short test")
	}
	t1 := make(TagSet, 10)
	t2 := make(TagSet, 10)
	for i := 0; i < len(t1); i++ {
		t1[i] = Tag{
			key:   fmt.Sprintf("k1%d", i),
			value: fmt.Sprintf("v1_%d", i),
		}
		t2[i] = Tag{
			key:   fmt.Sprintf("k2%d", i),
			value: fmt.Sprintf("v2_%d", i),
		}
	}
	t1.Sort()
	t2.Sort()

	scratch := make(TagSet, len(t1)+len(t2))

	b.ResetTimer()
	b.Run("KeysNotEqual", func(b *testing.B) {
		for size := 2; size <= 10; size += 2 {
			b.Run(fmt.Sprint(size), func(b *testing.B) {
				s1 := t1[:size]
				s2 := t2[:size]
				for i := 0; i < b.N; i++ {
					mergeTagSets(s1, s2, scratch)
				}
			})
		}
	})

	b.Run("KeysHalfEqual", func(b *testing.B) {
		for i := range t2 {
			if i&1 != 0 {
				t2[i].key = t1[i].key
			}
		}
		t2.Sort()
		for size := 2; size <= 10; size += 2 {
			b.Run(fmt.Sprint(size), func(b *testing.B) {
				s1 := t1[:size]
				s2 := t2[:size]
				for i := 0; i < b.N; i++ {
					mergeTagSets(s1, s2, scratch)
				}
			})
		}
	})

	b.Run("KeysEqual", func(b *testing.B) {
		for i := range t2 {
			t2[i].key = t1[i].key
		}
		for size := 2; size <= 10; size += 2 {
			b.Run(fmt.Sprint(size), func(b *testing.B) {
				s1 := t1[:size]
				s2 := t2[:size]
				for i := 0; i < b.N; i++ {
					mergeTagSets(s1, s2, scratch)
				}
			})
		}
	})
}

func BenchmarkTagSetSearch_Reference(b *testing.B) {
	var keys [5]string
	tags := randomTagSet(b, "v_", len(keys))
	for i := 0; i < len(tags); i++ {
		keys[i] = tags[i].key
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := keys[i%len(keys)]
		sort.Search(len(tags), func(i int) bool {
			return tags[i].key >= key
		})
	}
}

func BenchmarkTagSetSearch(b *testing.B) {
	var keys [5]string
	tags := randomTagSet(b, "v_", len(keys))
	for i := 0; i < len(tags); i++ {
		keys[i] = tags[i].key
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tags.Search(keys[i%len(keys)])
	}
}

///////////////////////////////////////////////////////////////////
// TAG SET TESTS !!!
///////////////////////////////////////////////////////////////////

func mergeTagsReference(set TagSet, tags map[string]string) TagSet {
	a := make(TagSet, 0, len(tags))
	for k, v := range tags {
		if k != "" && v != "" {
			a = append(a, Tag{key: k, value: replaceChars(v)})
		}
	}
	return mergeTagSetsReference(set, a)
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

func testMergeTags(t *testing.T, s1, s2 TagSet, perInstanceTag bool) {
	tags := make(map[string]string, len(s2))
	origTags := make(map[string]string, len(s2))
	for _, p := range s2 {
		tags[p.key] = p.value
		origTags[p.key] = p.value
	}

	set := s1
	origPairs := append(TagSet(nil), set...)
	expected := mergeTagsReference(set, tags)

	var got TagSet
	if perInstanceTag {
		if !expected.Contains("_f") {
			expected = expected.Insert(Tag{key: "_f", value: "i"})
		}
		got = mergePerInstanceTags(set, tags)
	} else {
		got = mergeTags(set, tags)
	}
	if !tagSetEqual(got, expected) {
		t.Errorf("{s1=%d, s2=%d}: bad merge:\n# Got:\n%+v\n\n# Want:\n%+v\n",
			len(s1), len(s2), got, expected)
	}
	if !tagSetEqual(set, origPairs) {
		t.Fatalf("scope tags modified:\n# Got:\n%+v\n\n# Want:\n%+v\n",
			set, origPairs)
	}
	if !tagMapsEqual(tags, origTags) {
		t.Fatalf("tag map modified:\n# Got:\n%v\n\n# Want:\n%v\n",
			set, origPairs)
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
			s1 = s1.Insert(Tag{key: "_f", value: "foo"})
		}
		if rr.Float64() < 0.1 {
			s1 = s1.Insert(Tag{key: "_f", value: "foo"})
		}
		if rr.Float64() < 0.1 {
			s2 = s2.Insert(Tag{key: "_f", value: "bar"})
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
	mergeOneTag(TagSet{}, tags)
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
	set := make(TagSet, baseSize)
	for i := range set {
		set[i] = Tag{
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
		mergeTags(set, tags)
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
