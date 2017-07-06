package stats

import (
	"bytes"
	"fmt"
	"sort"
)

const (
	tagPrefix = ".__"
	tagSep    = "="
)

type tagPair struct {
	dimension string
	value     string
}

type tagSet []tagPair

func (t tagSet) Len() int           { return len(t) }
func (t tagSet) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t tagSet) Less(i, j int) bool { return t[i].dimension < t[j].dimension }

func serializeTags(tags map[string]string) string {
	tagPairs := make([]tagPair, 0, len(tags))
	for tagKey, tagValue := range tags {
		tagPairs = append(tagPairs, tagPair{tagKey, tagValue})
	}
	sort.Sort(tagSet(tagPairs))

	buf := new(bytes.Buffer)
	for _, tag := range tagPairs {
		fmt.Fprint(buf, tagPrefix, tag.dimension, tagSep, tag.value)
	}
	return buf.String()
}
