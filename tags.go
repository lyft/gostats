package stats

import (
	"fmt"
	"sort"
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
	tagPairs := make([]tagPair, 0)
	for tagKey, tagValue := range tags {
		tagPairs = append(tagPairs, tagPair{tagKey, tagValue})
	}

	sort.Sort(tagSet(tagPairs))
	var output string
	for _, tag := range tagPairs {
		output = fmt.Sprintf("%s.__%s=%s", output, tag.dimension, tag.value)
	}
	return output
}
