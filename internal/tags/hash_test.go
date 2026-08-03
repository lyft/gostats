package tags

import (
	"fmt"
	"hash/maphash"
	"testing"
)

// referenceHash hashes the canonical serialized form produced by the
// existing (already well-tested) SerializeTags, as an independent oracle
// for HashTags.
func referenceHash(name string, tags map[string]string) (uint64, uint64) {
	s := SerializeTags(name, tags)
	b := []byte(s)
	return maphash.Bytes(hashSeed1, b), maphash.Bytes(hashSeed2, b)
}

func TestHashTagsMatchesSerializeTags(t *testing.T) {
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
		wantHi, wantLo := referenceHash(name, tags)
		gotHi, gotLo := HashTags(name, tags)
		if gotHi != wantHi || gotLo != wantLo {
			t.Errorf("%d: HashTags = (%x,%x), want (%x,%x)", i, gotHi, gotLo, wantHi, wantLo)
		}
	}
}

func TestHashTagsInvalidKeyValue(t *testing.T) {
	tags := map[string]string{
		"":              "invalid_key",
		"invalid_value": "",
		"1":             "1",
	}
	wantHi, wantLo := referenceHash("name", tags)
	gotHi, gotLo := HashTags("name", tags)
	if gotHi != wantHi || gotLo != wantLo {
		t.Errorf("HashTags = (%x,%x), want (%x,%x)", gotHi, gotLo, wantHi, wantLo)
	}
}

func TestTagSetHashMatchesSerialize(t *testing.T) {
	const name = "prefix"
	for i := 0; i < 100; i++ {
		tags := make(map[string]string, i)
		for j := 0; j < i; j++ {
			tags[fmt.Sprintf("key%d", j)] = fmt.Sprintf("val%d", j)
		}
		ts := NewTagSet(tags)
		s := ts.Serialize(name)
		wantHi, wantLo := maphash.Bytes(hashSeed1, []byte(s)), maphash.Bytes(hashSeed2, []byte(s))
		gotHi, gotLo := ts.Hash(name)
		if gotHi != wantHi || gotLo != wantLo {
			t.Errorf("%d: TagSet.Hash = (%x,%x), want (%x,%x)", i, gotHi, gotLo, wantHi, wantLo)
		}
	}
}

func TestHashTagsAllocs(t *testing.T) {
	tags := map[string]string{"region": "us-east-1", "az": "1a", "shard": "17"}
	n := testing.AllocsPerRun(1000, func() {
		HashTags("stat.name", tags)
	})
	if n > 0 {
		t.Errorf("expected 0 allocs for small tag set, got %v", n)
	}
}

func TestHashTagsOverflowAllocs(t *testing.T) {
	tags := make(map[string]string, 64)
	for i := 0; i < 64; i++ {
		tags[fmt.Sprintf("key%02d", i)] = fmt.Sprintf("value-%02d-xxxxx", i)
	}
	n := testing.AllocsPerRun(1000, func() {
		HashTags("stat.name", tags)
	})
	// one alloc for the pairs gather (>4 tags), one for sort.Sort's
	// interface boxing of TagSet (>8 tags; pre-existing cost, also paid
	// by SerializeTags's equivalent default branch), and one for the
	// heap overflow buffer (>512 bytes serialized).
	if n > 3 {
		t.Errorf("expected at most 3 allocs for oversized tag set, got %v", n)
	}
}
