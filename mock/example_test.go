package mock_test

import (
	"fmt"
	"reflect"

	stats "github.com/lyft/gostats"
	"github.com/lyft/gostats/mock"
)

func ExampleSerializeTags() {
	sink := mock.NewSink()
	store := stats.NewStore(sink, false)

	tags := map[string]string{
		"key_1": "val_1",
		"key_2": "val_2",
	}
	counter := store.NewCounterWithTags("counter", tags)
	counter.Add(2)

	store.Flush()

	n := sink.Counter(mock.SerializeTags("counter", tags))
	fmt.Println("counter:", n, n == 2)

	// Output:
	// counter: 2 true
}

func ExampleParseTags() {
	sink := mock.NewSink()
	store := stats.NewStore(sink, false)

	tags := map[string]string{
		"key_1": "val_1",
		"key_2": "val_2",
	}
	for i := 0; i < 4; i++ {
		store.NewCounterWithTags(fmt.Sprintf("c_%d", i), tags).Inc()
	}
	store.Flush()

	// Check that the counters all have the same tags
	for stat := range sink.Counters() {
		name, m := mock.ParseTags(stat)
		if !reflect.DeepEqual(m, tags) {
			panic(fmt.Sprintf("Tags: got: %q want: %q", m, tags))
		}
		fmt.Printf("%s: okay\n", name)
	}

	// Unordered output:
	// c_0: okay
	// c_1: okay
	// c_2: okay
	// c_3: okay
}

func ExampleFatal() {
	sink := mock.NewSink()
	store := stats.NewStore(sink, false)

	store.NewCounter("c").Set(2)
	store.Flush()

}
