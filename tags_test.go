package stats

import (
	"bufio"
	"bytes"
	crand "crypto/rand"
	"encoding/base64"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
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
	tagPairs := make([]tagPair, 0, len(tags))
	for tagKey, tagValue := range tags {
		tagValue = replaceChars(tagValue)
		tagPairs = append(tagPairs, tagPair{tagKey, tagValue})
	}
	sort.Sort(tagSet(tagPairs))

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
	contains := func(key string, tags tagSet) bool {
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

func randomTagSet(t testing.TB, valPrefix string, size int) tagSet {
	s := make(tagSet, size)
	for i := 0; i < len(s); i++ {
		s[i] = tagPair{
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

func mergeTagSetsReference(s1, s2 tagSet) tagSet {
	seen := make(map[string]bool)
	var a tagSet
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

func makeScratch(s1, s2 tagSet) (tagSet, tagSet) {
	a := make(tagSet, len(s1)+len(s2))
	copy(a[len(s1):], s2)
	return a, a[len(s1):]
}

func tagSetEqual(s1, s2 tagSet) bool {
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
	tags := make(tagSet, 0, n)
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("key%d", i)
		v := fmt.Sprintf("val%d", i)
		tags = append(tags, tagPair{
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
	tags := make(tagSet, 5)
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
	tags := make(tagSet, 5)
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
	orig := make(tagSet, size)
	for i := range orig {
		orig[i].key = strconv.FormatInt(rr.Int63(), 10)
		orig[i].value = strconv.FormatInt(rr.Int63(), 10)
	}
	rr.Shuffle(len(orig), func(i, j int) {
		orig[i], orig[j] = orig[j], orig[i]
	})
	tags := make(tagSet, len(orig))

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
	t1 := make(tagSet, 10)
	t2 := make(tagSet, 10)
	for i := 0; i < len(t1); i++ {
		t1[i] = tagPair{
			key:   fmt.Sprintf("k1%d", i),
			value: fmt.Sprintf("v1_%d", i),
		}
		t2[i] = tagPair{
			key:   fmt.Sprintf("k2%d", i),
			value: fmt.Sprintf("v2_%d", i),
		}
	}
	t1.Sort()
	t2.Sort()

	scratch := make(tagSet, len(t1)+len(t2))

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
