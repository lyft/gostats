package tags

import (
	"sort"
	"strings"
	"unsafe"
)

// A Tag is a Key/Value statsd tag.
type Tag struct {
	Key   string
	Value string
}

// NewTag returns a new Tag with any invalid chars in the value replaced.
func NewTag(key, value string) Tag {
	return Tag{Key: key, Value: ReplaceChars(value)}
}

// A TagSet is a collection of Tags. All methods apart from Sort() require the
// TagSet to be sorted. Tags with empty keys or values should never be inserted
// into the TagSet since most methods rely on all the tags being valid.
type TagSet []Tag

// NewTagSet returns a new TagSet from the tags map.
func NewTagSet(tags map[string]string) TagSet {
	a := make(TagSet, 0, len(tags))
	for k, v := range tags {
		if k != "" && v != "" {
			a = append(a, NewTag(k, v))
		}
	}
	a.Sort()
	return a
}

func (t TagSet) Len() int           { return len(t) }
func (t TagSet) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t TagSet) Less(i, j int) bool { return t[i].Key < t[j].Key }

// cas performs a compare and swap and is inlined into Sort()
// check with: `go build -gcflags='-m'`.
func (t TagSet) cas(i, j int) {
	if t[i].Key > t[j].Key {
		t.Swap(i, j)
	}
}

// Sort sorts the TagSet in place and is optimized for small (N <= 8) TagSets.
func (t TagSet) Sort() {
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
func (t TagSet) Search(key string) int {
	i, j := 0, len(t)
	for i < j {
		h := (i + j) / 2
		if t[h].Key < key {
			i = h + 1
		} else {
			j = h
		}
	}
	return i
}

// Contains returns if the TagSet contains key.
func (t TagSet) Contains(key string) bool {
	if len(t) == 0 {
		return false
	}
	i := t.Search(key)
	return i < len(t) && t[i].Key == key
}

// Insert inserts Tag p into TagSet t, returning a copy unless Tag p already
// existed.
func (t TagSet) Insert(p Tag) TagSet {
	if len(t) == 0 {
		return TagSet{p}
	}

	i := t.Search(p.Key)
	if i < len(t) && t[i].Key == p.Key {
		if t[i].Value == p.Value {
			return t // no change
		}
		a := make(TagSet, len(t))
		copy(a, t)
		a[i].Value = p.Value
		return a // exists
	}

	// we're modifying the set - make a copy
	a := make(TagSet, len(t)+1)
	copy(a[:i], t[:i])
	a[i] = p
	copy(a[i+1:], t[i:])
	return a
}

// MergeTags returns a TagSet that is the union of subScope's tags and the
// provided tags map. If any keys overlap the values from the provided map
// are used.
func (t TagSet) MergeTags(tags map[string]string) TagSet {
	switch len(tags) {
	case 0:
		return t
	case 1:
		// optimize for the common case of there only being one tag
		return mergeOneTag(t, tags)
	default:
		// write tags to the end of the scratch slice
		scratch := make(TagSet, len(t)+len(tags))
		a := scratch[len(t):]
		i := 0
		for k, v := range tags {
			if k != "" && v != "" {
				a[i] = NewTag(k, v)
				i++
			}
		}
		a = a[:i]
		a.Sort()

		if len(t) == 0 {
			return a
		}
		return mergeTagSets(t, a, scratch)
	}
}

// mergeOneTag is an optimized for inserting 1 tag and will panic otherwise.
func mergeOneTag(set TagSet, tags map[string]string) TagSet {
	if len(tags) != 1 {
		panic("invalid usage")
	}
	var p Tag
	for k, v := range tags {
		p = NewTag(k, v)
		break
	}
	if p.Key == "" || p.Value == "" {
		return set
	}
	return set.Insert(p)
}

// MergePerInstanceTags returns a TagSet that is the union of subScope's
// tags and the provided tags map with. If any keys overlap the values from
// the provided map are used.
//
// The returned TagSet will have a per-instance key ("_f") and if neither the
// subScope or tags have this key it's value will be the default per-instance
// value ("i").
//
// The method does not optimize for the case where there is only one tag
// because it is used less frequently.
func (t TagSet) MergePerInstanceTags(tags map[string]string) TagSet {
	if len(tags) == 0 {
		if t.Contains("_f") {
			return t
		}
		// create copy with the per-instance tag
		return t.Insert(Tag{Key: "_f", Value: "i"})
	}

	// write tags to the end of scratch slice
	scratch := make(TagSet, len(t)+len(tags)+1)
	a := scratch[len(t):]
	i := 0
	// add the default per-instance tag if not present
	if tags["_f"] == "" && !t.Contains("_f") {
		a[i] = Tag{Key: "_f", Value: "i"}
		i++
	}
	for k, v := range tags {
		if k != "" && v != "" {
			a[i] = NewTag(k, v)
			i++
		}
	}
	a = a[:i]
	a.Sort()

	if len(t) == 0 {
		return a
	}
	return mergeTagSets(t, a, scratch)
}

// mergeTagSets merges s1 into s2 and stores the result in scratch. Both s1 and
// s2 must be sorted and s2 can be a sub-slice of scratch if it is located at
// the tail of the slice (this allows us to allocate only one slice).
func mergeTagSets(s1, s2, scratch TagSet) TagSet {
	a := scratch
	i, j, k := 0, 0, 0
	for ; i < len(s1) && j < len(s2) && k < len(a); k++ {
		if s1[i].Key == s2[j].Key {
			a[k] = s2[j]
			i++
			j++
		} else if s1[i].Key < s2[j].Key {
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

// Serialize serializes name and tags into a statsd stat. Note: the TagSet
// t must be sorted and have clean tag keys and values.
func (t TagSet) Serialize(name string) string {
	// TODO: panic if the set isn't sorted?

	const prefix = ".__"
	const sep = "="

	if len(t) == 0 {
		return name
	}

	n := (len(prefix)+len(sep))*len(t) + len(name)
	for _, p := range t {
		n += len(p.Key) + len(p.Value)
	}

	// CEV: this is same as strings.Builder, but is faster and simpler.
	b := make([]byte, 0, n)
	b = append(b, name...)
	for _, p := range t {
		b = append(b, prefix...)
		b = append(b, p.Key...)
		b = append(b, sep...)
		b = append(b, p.Value...)
	}
	return *(*string)(unsafe.Pointer(&b))
}

// SerializeTags serializes name and tags into a statsd stat.
func SerializeTags(name string, tags map[string]string) string {
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
		var t0 Tag
		for k, v := range tags {
			if k != "" && v != "" {
				t0 = NewTag(k, v)
				break
			}
		}
		return name + prefix + t0.Key + sep + t0.Value
	case 2:
		var t0, t1 Tag
		for k, v := range tags {
			if k == "" || v == "" {
				continue
			}
			t1 = t0
			t0 = NewTag(k, v)
		}
		if t0.Key > t1.Key {
			t0, t1 = t1, t0
		}
		return name + prefix + t0.Key + sep + t0.Value +
			prefix + t1.Key + sep + t1.Value
	case 3:
		var t0, t1, t2 Tag
		for k, v := range tags {
			if k == "" || v == "" {
				continue
			}
			t2 = t1
			t1 = t0
			t0 = NewTag(k, v)
		}
		if t1.Key > t2.Key {
			t1, t2 = t2, t1
		}
		if t0.Key > t2.Key {
			t0, t2 = t2, t0
		}
		if t0.Key > t1.Key {
			t0, t1 = t1, t0
		}
		return name + prefix + t0.Key + sep + t0.Value +
			prefix + t1.Key + sep + t1.Value +
			prefix + t2.Key + sep + t2.Value
	case 4:
		var t0, t1, t2, t3 Tag
		for k, v := range tags {
			if k == "" || v == "" {
				continue
			}
			t3 = t2
			t2 = t1
			t1 = t0
			t0 = NewTag(k, v)
		}
		if t0.Key > t1.Key {
			t0, t1 = t1, t0
		}
		if t2.Key > t3.Key {
			t2, t3 = t3, t2
		}
		if t0.Key > t2.Key {
			t0, t2 = t2, t0
		}
		if t1.Key > t3.Key {
			t1, t3 = t3, t1
		}
		if t1.Key > t2.Key {
			t1, t2 = t2, t1
		}
		return name + prefix + t0.Key + sep + t0.Value +
			prefix + t1.Key + sep + t1.Value +
			prefix + t2.Key + sep + t2.Value +
			prefix + t3.Key + sep + t3.Value
	default:
		// n stores the length of the serialized name + tags
		n := (len(prefix) + len(sep)) * numValid
		n += len(name)

		pairs := make(TagSet, 0, numValid)
		for k, v := range tags {
			if k == "" || v == "" {
				continue
			}
			n += len(k) + len(v)
			pairs = append(pairs, NewTag(k, v))
		}
		sort.Sort(pairs)

		// CEV: this is same as strings.Builder, but works with go1.9 and earlier
		b := make([]byte, 0, n)
		b = append(b, name...)
		for _, tag := range pairs {
			b = append(b, prefix...)
			b = append(b, tag.Key...)
			b = append(b, sep...)
			b = append(b, tag.Value...)
		}
		return *(*string)(unsafe.Pointer(&b))
	}
}

// ReplaceChars replaces any invalid chars ([.:|]) in value s with '_'.
func ReplaceChars(s string) string {
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

// removeStatValue removes the value from a stat line
func removeStatValue(s string) string {
	i := strings.IndexByte(s, ':')
	if i == -1 {
		return s
	}
	n := i
	i++
	for i < len(s) {
		if s[i] == ':' {
			n = i
		}
		i++
	}
	return s[:n]
}

// ParseTags parses the statsd stat name and tags (if any) from stat.
func ParseTags(stat string) (string, map[string]string) {
	const sep = ".__"

	// Remove the value, if any. This allows passing full stat
	// lines in wire form as they would be emitted by the sink.
	stat = removeStatValue(stat)

	o := strings.Index(stat, sep)
	if o == -1 {
		return stat, nil // no tags
	}
	name := stat[:o]

	// consume first sep
	s := stat[o+len(sep):]

	n := strings.Count(s, sep)
	tags := make(map[string]string, n+1)

	for ; n > 0; n-- {
		m := strings.Index(s, sep)
		if m < 0 {
			break
		}
		a := s[:m]
		if o := strings.IndexByte(a, '='); o != -1 {
			tags[a[:o]] = a[o+1:]
		}
		s = s[m+len(sep):]
	}
	if o := strings.IndexByte(s, '='); o != -1 {
		tags[s[:o]] = s[o+1:]
	}
	return name, tags
}

// ParseTagSet parses the statsd stat name and tags (if any) from stat. It is
// like ParseTags, but returns a TagSet instead of a map[string]string.
func ParseTagSet(stat string) (string, TagSet) {
	const sep = ".__"

	// Remove the value, if any. This allows passing full stat
	// lines in wire form as they would be emitted by the sink.
	stat = removeStatValue(stat)

	o := strings.Index(stat, sep)
	if o == -1 {
		return stat, nil // no tags
	}
	name := stat[:o]

	// consume first sep
	s := stat[o+len(sep):]

	n := strings.Count(s, sep)
	tags := make(TagSet, n+1)

	i := 0
	for n > 0 {
		m := strings.Index(s, sep)
		if m < 0 {
			break
		}
		// TODO: handle malformed stats ???
		a := s[:m]
		if o := strings.IndexByte(a, '='); o != -1 {
			tags[i] = Tag{
				Key:   a[:o],
				Value: a[o+1:], // don't clean the stat
			}
			i++
		}
		s = s[m+len(sep):]
		n--
	}
	if o := strings.IndexByte(s, '='); o != -1 {
		tags[i] = Tag{
			Key:   s[:o],
			Value: s[o+1:],
		}
		i++
	}
	tags = tags[:i]
	tags.Sort()
	return name, tags
}
