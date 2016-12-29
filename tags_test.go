package stats

import (
	"testing"
)

func TestSerializeTags(t *testing.T) {
	tags := map[string]string{"zzz": "hello", "q": "r"}
	serialized := serializeTags(tags)
	if serialized != ".__q=r.__zzz=hello" {
		t.Errorf("Serialized output (%s) didn't match expected output", serialized)
	}
}
