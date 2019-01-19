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

func serializeTags(name string, tags map[string]string) string {
	const prefix = ".__"
	const sep = "="

	// switch len(tags) {
	switch len(tags) {
	case 0:
		return name
	case 1:
		for k, v := range tags {
			return name + prefix + k + sep + replaceChars(v)
		}
		panic("unreachable")
	case 2:
		var a, b tagPair
		for k, v := range tags {
			b = a
			a = tagPair{k, replaceChars(v)}
		}
		if a.dimension > b.dimension {
			a, b = b, a
		}
		return name + prefix + a.dimension + sep + a.value +
			prefix + b.dimension + sep + b.value
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
