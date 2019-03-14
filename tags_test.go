package stats

import (
	"bytes"
	"fmt"
	"sort"
	"testing"
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
		fmt.Fprint(buf, prefix, tag.dimension, sep, tag.value)
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
