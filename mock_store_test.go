package stats

import "testing"

func TestMockStore(t *testing.T) {
	store, sink := NewMockStore(t)
	_ = sink
	store.NewGaugeWithTags("foo", map[string]string{
		"bad": "",
		"xxx": "bar.foo",
	})
}
