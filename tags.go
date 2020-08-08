package stats

import (
	"sort"
	"unsafe"
)

type tagPair struct {
	key   string
	value string
}

type tagSet []tagPair

func (t tagSet) Len() int           { return len(t) }
func (t tagSet) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t tagSet) Less(i, j int) bool { return t[i].key < t[j].key }

// cas performs a compare and swap and is inlined into Sort()
// check with: `go build -gcflags='-m'`.
func (t tagSet) cas(i, j int) {
	if t[i].key > t[j].key {
		t.Swap(i, j)
	}
}

// Sort sorts the tagSet in place and is optimized for small (N <= 8) tagSets.
func (t tagSet) Sort() {
	// network sort generated with: https://pages.ripco.net/~jgamble/nw.html
	// using "best": https://metacpan.org/pod/Algorithm::Networksort::Best
	//
	// example query (N=6):
	//   http://jgamble.ripco.net/cgi-bin/nw.cgi?inputs=6&algorithm=best&output=macro
	//
	// all cas() methods are inlined, check with: `go build -gcflags='-m'`
	switch len(t) {
	case 0, 1:
		return
	case 2:
		t.cas(0, 1)
	case 3:
		t.cas(1, 2)
		t.cas(0, 2)
		t.cas(0, 1)
	case 4:
		t.cas(0, 1)
		t.cas(2, 3)
		t.cas(0, 2)
		t.cas(1, 3)
		t.cas(1, 2)
	case 5:
		t.cas(0, 1)
		t.cas(3, 4)
		t.cas(2, 4)
		t.cas(2, 3)
		t.cas(0, 3)
		t.cas(0, 2)
		t.cas(1, 4)
		t.cas(1, 3)
		t.cas(1, 2)
	case 6:
		t.cas(1, 2)
		t.cas(0, 2)
		t.cas(0, 1)
		t.cas(4, 5)
		t.cas(3, 5)
		t.cas(3, 4)
		t.cas(0, 3)
		t.cas(1, 4)
		t.cas(2, 5)
		t.cas(2, 4)
		t.cas(1, 3)
		t.cas(2, 3)
	case 7:
		t.cas(1, 2)
		t.cas(0, 2)
		t.cas(0, 1)
		t.cas(3, 4)
		t.cas(5, 6)
		t.cas(3, 5)
		t.cas(4, 6)
		t.cas(4, 5)
		t.cas(0, 4)
		t.cas(0, 3)
		t.cas(1, 5)
		t.cas(2, 6)
		t.cas(2, 5)
		t.cas(1, 3)
		t.cas(2, 4)
		t.cas(2, 3)
	case 8:
		t.cas(0, 1)
		t.cas(2, 3)
		t.cas(0, 2)
		t.cas(1, 3)
		t.cas(1, 2)
		t.cas(4, 5)
		t.cas(6, 7)
		t.cas(4, 6)
		t.cas(5, 7)
		t.cas(5, 6)
		t.cas(0, 4)
		t.cas(1, 5)
		t.cas(1, 4)
		t.cas(2, 6)
		t.cas(3, 7)
		t.cas(3, 6)
		t.cas(2, 4)
		t.cas(3, 5)
		t.cas(3, 4)
	default:
		sort.Sort(t)
	}
}

// Search is the same as sort.Search() but is optimized for our use case.
func (t tagSet) Search(key string) int {
	i, j := 0, len(t)
	for i < j {
		h := (i + j) / 2
		if t[h].key < key {
			i = h + 1
		} else {
			j = h
		}
	}
	return i
}

// Contains returns if the tagSet contains key.
func (t tagSet) Contains(key string) bool {
	if len(t) == 0 {
		return false
	}
	i := t.Search(key)
	return i < len(t) && t[i].key == key
}

// Inserts tagPair p into tagSet, if a tagPair with the same key exists it is
// replaced.
func (t tagSet) Insert(p tagPair) tagSet {
	i := t.Search(p.key)
	if i < len(t) && t[i].key == p.key {
		t[i].value = p.value
		return t // exists
	}
	// append t to the end of the slice
	if i == len(t) {
		return append(t, p)
	}
	// insert p
	t = append(t, tagPair{})
	copy(t[i+1:], t[i:])
	t[i] = p
	return t
}

// mergeTagSets merges s1 into s2 and stores the result in scratch. Both s1 and
// s2 must be sorted and s2 can be a sub-slice of scratch if it is located at
// the tail of the slice (this allows us to allocate only one slice).
func mergeTagSets(s1, s2, scratch tagSet) tagSet {
	a := scratch
	i, j, k := 0, 0, 0
	for ; i < len(s1) && j < len(s2) && k < len(a); k++ {
		if s1[i].key == s2[j].key {
			a[k] = s2[j]
			i++
			j++
		} else if s1[i].key < s2[j].key {
			a[k] = s1[i]
			i++
		} else {
			a[k] = s2[j]
			j++
		}
	}
	if i < len(s1) {
		k += copy(a[k:], s1[i:])
	}
	if j < len(s2) {
		k += copy(a[k:], s2[j:])
	}
	return a[:k]
}

func serializeTagSet(name string, set tagSet) string {
	// NB: the tagSet must be sorted and have clean values

	const prefix = ".__"
	const sep = "="

	if len(set) == 0 {
		return name
	}

	n := (len(prefix)+len(sep))*len(set) + len(name)
	for _, p := range set {
		n += len(p.key) + len(p.value)
	}

	// CEV: this is same as strings.Builder, but is faster and simpler.
	b := make([]byte, 0, n)
	b = append(b, name...)
	for _, p := range set {
		b = append(b, prefix...)
		b = append(b, p.key...)
		b = append(b, sep...)
		b = append(b, p.value...)
	}
	return *(*string)(unsafe.Pointer(&b))
}

func serializeTags(name string, tags map[string]string) string {
	const prefix = ".__"
	const sep = "="

	// discard pairs where the tag or value is an empty string
	numValid := len(tags)
	for k, v := range tags {
		if k == "" || v == "" {
			numValid--
		}
	}

	switch numValid {
	case 0:
		return name
	case 1:
		for k, v := range tags {
			if k != "" && v != "" {
				return name + prefix + k + sep + replaceChars(v)
			}
		}
		panic("unreachable")
	case 2:
		var t0, t1 tagPair
		for k, v := range tags {
			if k == "" || v == "" {
				continue
			}
			t1 = t0
			t0 = tagPair{k, replaceChars(v)}
		}
		if t0.key > t1.key {
			t0, t1 = t1, t0
		}
		return name + prefix + t0.key + sep + t0.value +
			prefix + t1.key + sep + t1.value
	case 3:
		var t0, t1, t2 tagPair
		for k, v := range tags {
			if k == "" || v == "" {
				continue
			}
			t2 = t1
			t1 = t0
			t0 = tagPair{k, replaceChars(v)}
		}
		if t1.key > t2.key {
			t1, t2 = t2, t1
		}
		if t0.key > t2.key {
			t0, t2 = t2, t0
		}
		if t0.key > t1.key {
			t0, t1 = t1, t0
		}
		return name + prefix + t0.key + sep + t0.value +
			prefix + t1.key + sep + t1.value +
			prefix + t2.key + sep + t2.value
	case 4:
		var t0, t1, t2, t3 tagPair
		for k, v := range tags {
			if k == "" || v == "" {
				continue
			}
			t3 = t2
			t2 = t1
			t1 = t0
			t0 = tagPair{k, replaceChars(v)}
		}
		if t0.key > t1.key {
			t0, t1 = t1, t0
		}
		if t2.key > t3.key {
			t2, t3 = t3, t2
		}
		if t0.key > t2.key {
			t0, t2 = t2, t0
		}
		if t1.key > t3.key {
			t1, t3 = t3, t1
		}
		if t1.key > t2.key {
			t1, t2 = t2, t1
		}
		return name + prefix + t0.key + sep + t0.value +
			prefix + t1.key + sep + t1.value +
			prefix + t2.key + sep + t2.value +
			prefix + t3.key + sep + t3.value
	default:
		// n stores the length of the serialized name + tags
		n := (len(prefix) + len(sep)) * numValid
		n += len(name)

		pairs := make(tagSet, 0, numValid)
		for k, v := range tags {
			if k == "" || v == "" {
				continue
			}
			n += len(k) + len(v)
			pairs = append(pairs, tagPair{
				key:   k,
				value: replaceChars(v),
			})
		}
		sort.Sort(pairs)

		// CEV: this is same as strings.Builder, but works with go1.9 and earlier
		b := make([]byte, 0, n)
		b = append(b, name...)
		for _, tag := range pairs {
			b = append(b, prefix...)
			b = append(b, tag.key...)
			b = append(b, sep...)
			b = append(b, tag.value...)
		}
		return *(*string)(unsafe.Pointer(&b))
	}
}

func replaceChars(s string) string {
	var buf []byte // lazily allocated
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.', ':', '|':
			if buf == nil {
				buf = []byte(s)
			}
			buf[i] = '_'
		}
	}
	if buf == nil {
		return s
	}
	return *(*string)(unsafe.Pointer(&buf))
}
