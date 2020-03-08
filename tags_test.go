package stats

import (
	"bufio"
	"bytes"
	crand "crypto/rand"
	"encoding/base64"
	"fmt"
	"math/rand"
	"sort"
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
		if tag.dimension != "" && tag.value != "" {
			fmt.Fprint(buf, prefix, tag.dimension, sep, tag.value)
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

func TestSerializeTag(t *testing.T) {
	const (
		prefix = ".__"
		sep    = "="
		name   = "x"
	)
	if s := serializeTag(name, "", "value"); s != name {
		t.Errorf("serializeTag: got: %q want: %q", s, name)
	}
	if s := serializeTag(name, "key", ""); s != name {
		t.Errorf("serializeTag: got: %q want: %q", s, name)
	}

	exp := name + prefix + "key" + sep + replaceChars("value")
	if s := serializeTag(name, "key", "value"); s != exp {
		t.Errorf("serializeTag: got: %q want: %q", s, exp)
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

func BenchmarkSerializeTag(b *testing.B) {
	const (
		name  = "prefix"
		key   = "key1"
		value = "value1"
	)
	for i := 0; i < b.N; i++ {
		serializeTag(name, key, value)
	}
}

func BenchmarkSerializeTags(b *testing.B) {
	for i := 1; i <= 10; i++ {
		b.Run(fmt.Sprintf("%d", i), func(b *testing.B) {
			benchmarkSerializeTags(b, i)
		})
	}
}
