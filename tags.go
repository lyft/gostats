package stats

import (
	"sort"
	"strings"
	"unsafe"
)

// illegalTagValueChars are loosely set to ensure we don't have statsd parse errors.
var illegalTagValueCharsReplacer = strings.NewReplacer(
	".", "_",
	":", "_",
	"|", "_",
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

	if len(tags) == 0 {
		return name
	}

	// n stores the length of the serialized name + tags
	n := (len(prefix) + len(sep)) * len(tags)
	n += len(name)

	pairs := make([]tagPair, 0, len(tags))
	for k, v := range tags {
		n += len(k) + len(v)
		pairs = append(pairs, tagPair{
			dimension: k,
			value:     illegalTagValueCharsReplacer.Replace(v),
		})
	}
	sort.Sort(tagSet(pairs))

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
