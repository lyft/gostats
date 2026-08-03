package tags

import "hash/maphash"

// Two independently-seeded hashes are combined into a 128-bit key. The
// seeds are randomized once per process (see hash/maphash), which is fine
// since the result is only ever used as an in-memory map key, never
// persisted or compared across processes.
var (
	hashSeed1 = maphash.MakeSeed()
	hashSeed2 = maphash.MakeSeed()
)

// HashTags returns a 128-bit hash that uniquely identifies the canonical
// serialized form of name+tags -- the same string SerializeTags(name, tags)
// would produce, tags sorted the same way and pairs with an empty key or
// value discarded the same way -- without allocating for the common case.
//
// Unlike SerializeTags, HashTags never needs to materialize the serialized
// string itself, so for name+tags that serialize to 512 bytes or less (the
// overwhelming majority in practice) it performs zero heap allocations.
// Longer serialized forms fall back to a single exact-size heap allocation.
func HashTags(name string, tags map[string]string) (hi, lo uint64) {
	numValid := len(tags)
	for k, v := range tags {
		if k == "" || v == "" {
			numValid--
		}
	}
	if numValid == 0 {
		return hashSerialize(name, nil)
	}

	// Gather into a small stack array for the common case; only tag sets
	// larger than this need a heap allocation for the gather itself.
	var arr [16]Tag
	var pairs TagSet
	if numValid <= len(arr) {
		pairs = arr[:0]
	} else {
		pairs = make(TagSet, 0, numValid)
	}
	for k, v := range tags {
		if k != "" && v != "" {
			pairs = append(pairs, NewTag(k, v))
		}
	}
	pairs.Sort()
	return hashSerialize(name, pairs)
}

// Hash returns a 128-bit hash that uniquely identifies the canonical
// serialized form of name+t (the same string t.Serialize(name) would
// produce). t must already be sorted, as with all other TagSet methods.
// It performs zero heap allocations as long as the serialized form is 512
// bytes or less.
func (t TagSet) Hash(name string) (hi, lo uint64) {
	return hashSerialize(name, t)
}

// hashSerialize builds the canonical ".__key=value"-joined serialized form
// of name+pairs into a stack-resident buffer sized to fit the common case,
// falling back to a single exact-size heap allocation if needed, and hashes
// the result. It never returns or retains the buffer, so as long as the
// stack tier is used the buffer itself never escapes to the heap.
func hashSerialize(name string, pairs []Tag) (hi, lo uint64) {
	const prefix = ".__"
	const sep = "="

	n := len(name)
	for _, p := range pairs {
		n += len(prefix) + len(sep) + len(p.Key) + len(p.Value)
	}

	switch {
	case n <= 128:
		var arr [128]byte
		b := appendSerialized(arr[:0], name, pairs)
		return hashBytes(b)
	case n <= 256:
		var arr [256]byte
		b := appendSerialized(arr[:0], name, pairs)
		return hashBytes(b)
	case n <= 512:
		var arr [512]byte
		b := appendSerialized(arr[:0], name, pairs)
		return hashBytes(b)
	default:
		b := appendSerialized(make([]byte, 0, n), name, pairs)
		return hashBytes(b)
	}
}

func appendSerialized(b []byte, name string, pairs []Tag) []byte {
	b = append(b, name...)
	for _, p := range pairs {
		b = append(b, '.', '_', '_')
		b = append(b, p.Key...)
		b = append(b, '=')
		b = append(b, p.Value...)
	}
	return b
}

func hashBytes(b []byte) (hi, lo uint64) {
	return maphash.Bytes(hashSeed1, b), maphash.Bytes(hashSeed2, b)
}
