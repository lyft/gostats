package stats

import (
	"sort"
	"unsafe"
)

type tagPair struct {
	dimension string
	value     string
}

type tagSet []tagPair

func (t tagSet) Len() int           { return len(t) }
func (t tagSet) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t tagSet) Less(i, j int) bool { return t[i].dimension < t[j].dimension }

func fixTags(tags map[string]string) map[string]string {
	noEmpty := true
	for _, v := range tags {
		if v == "" {
			noEmpty = false
			break
		}
	}
	if noEmpty {
		return tags
	}
	newTags := make(map[string]string)
	for k, v := range tags {
		if v != "" {
			newTags[k] = v
		}
	}
	return newTags
}

func serializeTags(name string, tags map[string]string) string {
	const prefix = ".__"
	const sep = "="

	tags = fixTags(tags)

	switch len(tags) {
	case 0:
		return name
	case 1:
		for k, v := range tags {
			return name + prefix + k + sep + replaceChars(v)
		}
		panic("unreachable")
	case 2:
		var t0, t1 tagPair
		for k, v := range tags {
			t1 = t0
			t0 = tagPair{k, replaceChars(v)}
		}
		if t0.dimension > t1.dimension {
			t0, t1 = t1, t0
		}
		return name + prefix + t0.dimension + sep + t0.value +
			prefix + t1.dimension + sep + t1.value
	case 3:
		var t0, t1, t2 tagPair
		for k, v := range tags {
			t2 = t1
			t1 = t0
			t0 = tagPair{k, replaceChars(v)}
		}
		if t1.dimension > t2.dimension {
			t1, t2 = t2, t1
		}
		if t0.dimension > t2.dimension {
			t0, t2 = t2, t0
		}
		if t0.dimension > t1.dimension {
			t0, t1 = t1, t0
		}
		return name + prefix + t0.dimension + sep + t0.value +
			prefix + t1.dimension + sep + t1.value +
			prefix + t2.dimension + sep + t2.value
	case 4:
		var t0, t1, t2, t3 tagPair
		for k, v := range tags {
			t3 = t2
			t2 = t1
			t1 = t0
			t0 = tagPair{k, replaceChars(v)}
		}
		if t0.dimension > t1.dimension {
			t0, t1 = t1, t0
		}
		if t2.dimension > t3.dimension {
			t2, t3 = t3, t2
		}
		if t0.dimension > t2.dimension {
			t0, t2 = t2, t0
		}
		if t1.dimension > t3.dimension {
			t1, t3 = t3, t1
		}
		if t1.dimension > t2.dimension {
			t1, t2 = t2, t1
		}
		return name + prefix + t0.dimension + sep + t0.value +
			prefix + t1.dimension + sep + t1.value +
			prefix + t2.dimension + sep + t2.value +
			prefix + t3.dimension + sep + t3.value
	default:
		// n stores the length of the serialized name + tags
		n := (len(prefix) + len(sep)) * len(tags)
		n += len(name)

		pairs := make(tagSet, 0, len(tags))
		for k, v := range tags {
			n += len(k) + len(v)
			pairs = append(pairs, tagPair{
				dimension: k,
				value:     replaceChars(v),
			})
		}
		sort.Sort(pairs)

		// CEV: this is same as strings.Builder, but works with go1.9 and earlier
		b := make([]byte, 0, n)
		b = append(b, name...)
		for _, tag := range pairs {
			b = append(b, prefix...)
			b = append(b, tag.dimension...)
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
	return string(buf)
}
