package stats

import "testing"

func TestLookup(t *testing.T) {
	r := newRegistry()

	if r.tree.Find("a", "b").Find("c") != r.tree.Find("a", "b", "c") {
		t.FailNow()
	}
}
